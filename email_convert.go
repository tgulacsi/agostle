// Copyright 2017, 2022 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

// Needed: /email/convert?splitted=1&errors=1&id=xxx Accept: images/gif
//  /pdf/merge Accept: application/zip

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"context"

	"github.com/UNO-SOFT/filecache"
	"github.com/mholt/archiver/v4"
	"github.com/tgulacsi/agostle/converter"
	"github.com/tgulacsi/go/iohlp"

	kithttp "github.com/go-kit/kit/transport/http"
)

var emailConvertServer = kithttp.NewServer(
	emailConvertEP,
	emailConvertDecode,
	emailConvertEncode,
	kithttp.ServerBefore(defaultBeforeFuncs...),
	kithttp.ServerAfter(kithttp.SetContentType("application/zip")),
)

type convertParams struct {
	ContentType, OutImg, ImgSize string
	Pages                        []uint16
	Splitted, Merged             bool
}

func (p convertParams) String() string {
	var buf strings.Builder
	n := 5 + (len(p.ContentType) + 1) + (len(p.OutImg) + 1) + len(p.ImgSize) + 2 + 2*len(p.Pages)
	if n&(n-1) != 0 {
		// power of 2
		n = 1 << (bits.Len(uint(n)) + 1)
	}
	buf.Grow(n)
	w64 := func(s string) {
		if s == "" {
			return
		}
		b64 := base64.NewEncoder(base64.URLEncoding, &buf)
		_, _ = b64.Write([]byte(s))
		_ = b64.Close()
	}
	w64(p.ContentType)
	buf.WriteByte('_')
	w64(p.OutImg)
	buf.WriteByte('_')
	buf.WriteString(p.ImgSize)
	buf.WriteByte('_')
	if p.Splitted {
		buf.WriteByte('s')
	} else {
		buf.WriteByte('m')
	}
	buf.WriteByte('_')
	if p.Merged {
		buf.WriteByte('m')
	} else {
		buf.WriteByte('s')
	}
	if len(p.Pages) != 0 {
		buf.WriteByte('_')
		var b []byte
		for i, pg := range p.Pages {
			if i != 0 {
				buf.WriteByte(',')
			}
			b = strconv.AppendUint(b[:0], uint64(pg), 10)
			buf.Write(b)
		}
	}
	return buf.String()
}

var etagRe = regexp.MustCompile(`"[^"]+"`)

type emailConvertRequest struct {
	Params      convertParams
	Input       reqFile
	IfNoneMatch []string
	r           *http.Request
}

func emailConvertDecode(ctx context.Context, r *http.Request) (interface{}, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}
	req := emailConvertRequest{r: r, Params: convertParams{
		OutImg:  r.Form.Get("outimg"),
		ImgSize: r.Form.Get("imgsize"),
		Pages:   parseUint16s(r.Form["page"]),
		Merged:  r.Form.Get("merged") == "1" || r.Header.Get("Accept") == "application/pdf",
	}}
	req.Params.Splitted = len(req.Params.Pages) != 0 || r.Form.Get("splitted") == "1"
	if req.Params.ImgSize == "" {
		req.Params.ImgSize = defaultImageSize
	} else if strings.IndexByte(req.Params.ImgSize, 'x') < 0 {
		req.Params.ImgSize += "x" + req.Params.ImgSize
	}
	for _, a := range r.Header["Accept"] {
		if strings.HasPrefix(a, "image/") {
			req.Params.OutImg = a
			break
		}
	}
	inp, err := getOneRequestFile(ctx, r)
	if err != nil {
		return nil, err
	}
	req.Input = inp
	getLogger(ctx).Info("emailConvertDecode", "input", req.Input)
	contentType := req.Input.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		if strings.HasPrefix(r.URL.Path, "/convert") {
			contentType = ""
		} else {
			contentType = "message/rfc822"
		}
	}
	req.Params.ContentType = contentType

	// shortcut for ETag
	if etag := r.Header.Get("Etag"); etag != "" {
		if match := r.Header.Get("If-None-Match"); match != "" {
			req.IfNoneMatch = etagRe.FindAllString(match, -1)
		}
	}

	return req, nil
}

func emailConvertEP(ctx context.Context, request interface{}) (response interface{}, err error) {
	logger := getLogger(ctx).With("f", "emailConvertEP")
	req := request.(emailConvertRequest)
	defer func() { _ = req.Input.Close() }()

	getOutFn := func(params convertParams, hsh string) string {
		return filepath.Join(converter.Workdir,
			fmt.Sprintf("result-%s!%s.zip", hsh, params))
	}

	cacheKey := func(fn string) filecache.ActionID {
		return filecache.NewActionID([]byte(filepath.Base(fn)))
	}
	getCached := func(params convertParams, hsh string) (*os.File, error) {
		if strings.Contains(hsh, "/") {
			return nil, fmt.Errorf("bad hsh: %q", hsh)
		}
		outFn := getOutFn(params, hsh)
		var outFh *os.File
		var alreadyCached bool
		key := cacheKey(outFn)
		if fn, _, err := converter.Cache.GetFile(key); err == nil {
			var outErr error
			if outFh, outErr = os.Open(fn); outErr != nil {
				outFh = nil
			} else {
				alreadyCached = true
			}
		}
		if outFh == nil {
			var outErr error
			if outFh, outErr = os.Open(outFn); outErr != nil {
				return nil, outErr
			}
			defer func() {
				if err != nil {
					outFh.Close()
					logger.Info("Removing stale result", "file", outFn)
					_ = os.Remove(outFn)
				}
			}()
		}

		fi, statErr := outFh.Stat()
		if statErr != nil || fi.Size() == 0 {
			if statErr == nil {
				statErr = errors.New("zero file")
			}
			return nil, statErr
		}
		// test correctness of the zip file
		z, zErr := zip.OpenReader(outFh.Name())
		if zErr != nil {
			return nil, zErr
		}
		_ = z.Close()

		if !alreadyCached {
			_, _, _ = converter.Cache.Put(key, outFh)
			_, err := outFh.Seek(0, 0)
			return outFh, err
		}
		return outFh, nil
	}

	resp := emailConvertResponse{r: req.r}

	for _, hsh := range req.IfNoneMatch {
		var fh *os.File
		if fh, err = getCached(req.Params, hsh); err == nil {
			fh.Close()
			resp.NotModified = true
			return resp, nil
		}
	}

	h := sha256.New()
	sr, err := iohlp.MakeSectionReader(io.TeeReader(req.Input, h), 1<<20)
	logger.Info("readerToFile", "error", err)
	if err != nil {
		return resp, fmt.Errorf("cannot read input file: %w", err)
	}
	hsh := base64.URLEncoding.EncodeToString(h.Sum(nil))
	resp.outFn = getOutFn(req.Params, hsh)
	var head [1024]byte
	n, _ := sr.ReadAt(head[:], 0)
	req.Params.ContentType = converter.FixContentType(head[:n], req.Params.ContentType, req.Input.Filename)
	logger.Info("fixed", "params", req.Params)
	if fh, err := getCached(req.Params, hsh); err == nil {
		resp.outFn, resp.content = fh.Name(), fh
		logger.Info("use cached", "file", resp.outFn)
		err = resp.mergeIfRequested(ctx, req.Params)
		return resp, err
	}
	input := io.NewSectionReader(sr, 0, sr.Size())
	if !req.Params.Splitted && req.Params.OutImg == "" {
		err = converter.MailToPdfZip(ctx, resp.outFn, input, req.Params.ContentType)
		logger.Info("MailToPdfZip from", "from", input, "out", resp.outFn, "params", req.Params, "error", err)
		if err == nil {
			err = resp.mergeIfRequested(ctx, req.Params)
			logger.Info("mergeIfRequested", "error", err)
		}
	} else {
		err = converter.MailToSplittedPdfZip(ctx, resp.outFn, input, req.Params.ContentType,
			req.Params.Splitted, req.Params.OutImg, req.Params.ImgSize,
			req.Params.Pages)
		logger.Info("MailToSplittedPdfZip from", "from", input, "out", resp.outFn, "params", req.Params, "error", err)
	}
	if err == nil {
		var fh *os.File
		if fh, err = os.Open(resp.outFn); err == nil {
			_, _, _ = converter.Cache.Put(cacheKey(resp.outFn), fh)
			if _, err = fh.Seek(0, 0); err == nil {
				resp.content = fh
			}
		}
	}
	logger.Info("end", "contentNil", resp.content == nil, "fn", resp.outFn, "error", err)
	return resp, err
}

type readSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

type emailConvertResponse struct {
	content     readSeekCloser
	outFn, hsh  string
	r           *http.Request
	NotModified bool
}

func emailConvertEncode(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	logger := getLogger(ctx)
	resp, ok := response.(emailConvertResponse)
	if !ok {
		return fmt.Errorf("wanted emailConvertResponse, got %T", response)
	}
	logger.Info("emailConvertEncode", "notModified", resp.NotModified, "fn", resp.outFn, "contentNil", resp.content == nil)
	if resp.NotModified {
		w.WriteHeader(http.StatusNotModified)
	} else {
		w.Header().Set("Cache-Control", "max-age=2592000") // 30 days
		w.Header().Set("Etag", `"`+resp.hsh+`"`)
		if resp.content != nil {
			defer resp.content.Close()
			modTime := time.Now()
			if fi, err := os.Stat(resp.outFn); err == nil {
				modTime = fi.ModTime()
			}
			w.Header().Set("Content-Type", "application/pdf")
			http.ServeContent(w, resp.r, resp.outFn+".pdf", modTime, resp.content)
		} else {
			http.ServeFile(w, resp.r, resp.outFn)
		}
	}

	return nil
}

func (resp *emailConvertResponse) mergeIfRequested(ctx context.Context, params convertParams) error {
	if !params.Merged {
		return nil
	}
	fh, err := os.Open(resp.outFn)
	if err != nil {
		return err
	}
	defer fh.Close()
	format, _, err := archiver.Identify(fh.Name(), fh)
	if err != nil {
		return err
	}
	_, _ = fh.Seek(0, 0)
	var mr pdfMergeRequest
	defer func() {
		for _, inp := range mr.Inputs {
			if rc := inp.ReadCloser; rc != nil {
				rc.Close()
			}
		}
	}()
	A := func(name string, open func() (io.ReadCloser, error)) error {
		if !strings.HasSuffix(name, ".pdf") {
			return nil
		}
		rc, err := open()
		if err != nil {
			return err
		}
		defer rc.Close()
		sr, err := iohlp.MakeSectionReader(rc, 1<<20)
		if err != nil {
			return err
		}
		mr.Inputs = append(mr.Inputs, reqFile{
			FileHeader: multipart.FileHeader{Filename: name, Size: sr.Size()},
			ReadCloser: struct {
				io.Reader
				io.Closer
			}{sr, io.NopCloser(nil)},
		})
		return nil
	}

	if ex, ok := format.(archiver.Extractor); ok {
		if err = ex.Extract(ctx, fh, nil, func(ctx context.Context, f archiver.File) error {
			return A(f.Name(), f.Open)
		}); err != nil {
			return err
		}
	} else if decom, ok := format.(archiver.Decompressor); ok {
		nm := fh.Name()
		if err = A(nm[:len(nm)-len(filepath.Ext(nm))], func() (io.ReadCloser, error) { return decom.OpenReader(fh) }); err != nil {
			return err
		}
	}

	f, err := pdfMergeEP(ctx, mr)
	if err != nil {
		return fmt.Errorf("merge %v: %w", mr.Inputs, err)
	}
	resp.content = f.(readSeekCloser)
	return nil
}

func parseUint16s(ss []string) []uint16 {
	us := make([]uint16, 0, len(ss))
	for _, s := range ss {
		u, _ := strconv.ParseUint(s, 10, 16)
		if u != 0 {
			us = append(us, uint16(u))
		}
	}
	return us
}
