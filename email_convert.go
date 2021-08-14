// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

// Needed: /email/convert?splitted=1&errors=1&id=xxx Accept: images/gif
//  /pdf/merge Accept: application/zip

import (
	"archive/zip"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/bits"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"context"

	"github.com/go-kit/kit/log"
	"github.com/mholt/archiver"
	"github.com/tgulacsi/agostle/converter"

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
	buf.WriteString(strings.ReplaceAll(p.ContentType, "/", "--"))
	buf.WriteByte('_')
	buf.WriteString(strings.ReplaceAll(p.OutImg, "/", "--"))
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
	r.ParseForm()
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
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
	var err error
	req.Input, err = getOneRequestFile(ctx, r)
	if err != nil {
		return nil, err
	}
	//getLogger(ctx).Log("input", req.Input)
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
	logger := getLogger(ctx)
	Log := log.With(logger, "f", "emailConvertEP").Log
	req := request.(emailConvertRequest)
	defer func() { _ = req.Input.Close() }()

	getOutFn := func(params convertParams, hsh string) string {
		return filepath.Join(converter.Workdir,
			fmt.Sprintf("result-%s!%s.zip", hsh, params))
	}

	getCached := func(params convertParams, hsh string) (*os.File, error) {
		if strings.Contains(hsh, "/") {
			return nil, fmt.Errorf("bad hsh: %q", hsh)
		}
		outFn := getOutFn(params, hsh)
		converter.WorkdirMu.RLock()
		outFh, outErr := os.Open(outFn)
		if outErr == nil {
			now := time.Now()
			_ = os.Chtimes(outFh.Name(), now, now)
		}
		converter.WorkdirMu.RUnlock()
		if outErr != nil {
			return nil, outErr
		}
		defer func() {
			if err != nil {
				outFh.Close()
				Log("msg", "Removing stale result", "file", outFn)
				_ = os.Remove(outFn)
			}
		}()

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

	h := sha1.New()
	F := firstN{Data: make([]byte, 0, 1024)}
	inpFh, err := readerToFile(io.TeeReader(req.Input, io.MultiWriter(h, &F)), req.Input.Filename)
	Log("msg", "readerToFile", "error", err)
	if err != nil {
		return resp, fmt.Errorf("cannot read input file: %v", err)
	}
	defer func() { _ = inpFh.Cleanup() }()
	req.Params.ContentType = converter.FixContentType(F.Data, req.Params.ContentType, req.Input.Filename)
	Log("msg", "fixed", "params", req.Params)
	hsh := base64.URLEncoding.EncodeToString(h.Sum(nil))
	if fh, err := getCached(req.Params, hsh); err == nil {
		resp.outFn, resp.content = fh.Name(), fh
		Log("msg", "used cached", "file", resp.outFn)
		err = resp.mergeIfRequested(ctx, req.Params, logger)
		return resp, err
	}
	resp.outFn = getOutFn(req.Params, hsh)
	input := io.Reader(inpFh)
	if !req.Params.Splitted && req.Params.OutImg == "" {
		err = converter.MailToPdfZip(ctx, resp.outFn, input, req.Params.ContentType)
		Log("msg", "MailToPdfZip from", "from", input, "out", resp.outFn, "params", req.Params, "error", err)
		if err == nil {
			err = resp.mergeIfRequested(ctx, req.Params, logger)
			Log("msg", "mergeIfRequested", "error", err)
		}
	} else {
		err = converter.MailToSplittedPdfZip(ctx, resp.outFn, input, req.Params.ContentType,
			req.Params.Splitted, req.Params.OutImg, req.Params.ImgSize,
			req.Params.Pages)
		Log("msg", "MailToSplittedPdfZip from", "from", input, "out", resp.outFn, "params", req.Params, "error", err)
	}
	if err == nil {
		resp.content, err = os.Open(resp.outFn)
	}
	Log("msg", "end", "contentNil", resp.content == nil, "fn", resp.outFn, "error", err)
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
	Log := logger.Log
	resp, ok := response.(emailConvertResponse)
	if !ok {
		return fmt.Errorf("wanted emailConvertResponse, got %T", response)
	}
	Log("msg", "emailConvertEncode", "notModified", resp.NotModified, "fn", resp.outFn, "contentNil", resp.content == nil)
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

	converter.WorkdirMu.RLock()
	var tbd []string
	threshold := time.Now().Add(-7 * 24 * time.Hour)
	ds, _ := os.ReadDir(converter.Workdir)
	for _, d := range ds {
		if !d.Type().IsRegular() {
			continue
		}
		nm := d.Name()
		if !(strings.HasPrefix(nm, "result-") && strings.Contains(nm, "!") && strings.HasSuffix(nm, ".zip")) {
			continue
		}
		if fi, err := d.Info(); err != nil {
			_ = os.Remove(filepath.Join(converter.Workdir, nm))
			continue
		} else if fi.ModTime().Before(threshold) {
			tbd = append(tbd, filepath.Join(converter.Workdir, nm))
		}
	}
	converter.WorkdirMu.RUnlock()
	if len(tbd) == 0 {
		return nil
	}

	converter.WorkdirMu.Lock()
	defer converter.WorkdirMu.Unlock()
	for _, nm := range tbd {
		Log("msg", "remove", "file", nm)
		_ = os.Remove(nm)
	}
	return nil
}

func (resp *emailConvertResponse) mergeIfRequested(ctx context.Context, params convertParams, logger log.Logger) error {
	if !params.Merged {
		return nil
	}
	// merge PDFs
	tempDir, err := ioutil.TempDir("", "agostle-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	// archiver.Unarchive
	uaIface, err := archiver.ByExtension(resp.outFn)
	if err != nil {
		return err
	}
	u, ok := uaIface.(archiver.Unarchiver)
	if !ok {
		return fmt.Errorf("format specified by source filename is not an archive format: %s (%T)", resp.outFn, uaIface)
	}
	ru := reflect.ValueOf(u).Elem()
	if f := ru.FieldByName("MkdirAll"); f.IsValid() {
		f.SetBool(true)
	}
	if f := ru.FieldByName("OverwriteExisting"); f.IsValid() {
		f.SetBool(true)
	}
	if err = u.Unarchive(resp.outFn, tempDir); err != nil {
		return fmt.Errorf("unarchive %q to %q: %w", resp.outFn, tempDir, err)
	}

	dh, err := os.Open(tempDir)
	if err != nil {
		return fmt.Errorf("%s: %w", tempDir, err)
	}
	names, _ := dh.Readdirnames(-1)
	dh.Close()
	mr := pdfMergeRequest{Inputs: make([]reqFile, 0, len(names))}
	defer func() {
		for _, inp := range mr.Inputs {
			if rc := inp.ReadCloser; rc != nil {
				rc.Close()
			}
		}
	}()

	logger.Log("tempDir", tempDir, "files", names)
	for _, fn := range names {
		if strings.HasSuffix(fn, ".pdf") {
			fh, err := os.Open(filepath.Join(tempDir, fn))
			if err != nil {
				logger.Log("msg", "open", "file", filepath.Join(tempDir, fn), "error", err)
				continue
			}
			fi, _ := fh.Stat()
			mr.Inputs = append(mr.Inputs, reqFile{FileHeader: multipart.FileHeader{Filename: fh.Name(), Size: fi.Size()}, ReadCloser: fh})
		}
	}
	f, err := pdfMergeEP(ctx, mr)
	if err != nil {
		return fmt.Errorf("merge %v: %w", mr.Inputs, err)
	}
	resp.content = f.(readSeekCloser)
	return nil
}

type firstN struct {
	Data []byte
}

func (first *firstN) Write(p []byte) (int, error) {
	m := cap(first.Data) - len(first.Data)
	if m > 0 {
		if m > len(p) {
			m = len(p)
		}
		first.Data = append(first.Data, p[:m]...)
	}
	return len(p), nil
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
