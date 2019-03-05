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
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"context"

	"github.com/go-kit/kit/log"
	"github.com/mholt/archiver"
	"github.com/pkg/errors"
	"github.com/tgulacsi/agostle/converter"

	kithttp "github.com/go-kit/kit/transport/http"
)

var emailConvertServer = kithttp.NewServer(
	emailConvertEP,
	emailConvertDecode,
	emailConvertEncode,
	kithttp.ServerBefore(append([]kithttp.RequestFunc{SaveRequest}, defaultBeforeFuncs...)...),
	kithttp.ServerAfter(kithttp.SetContentType("application/zip")),
)

type convertParams struct {
	ContentType, OutImg, ImgSize string
	Splitted, Merged             bool
}

func (p convertParams) String() string {
	c := "m"
	if p.Splitted {
		c = "s"
	}
	m := "s"
	if p.Merged {
		m = "m"
	}
	return strings.Replace(p.ContentType, "/", "--", -1) + "_" + strings.Replace(p.OutImg, "/", "--", -1) + "_" + p.ImgSize + "_" + c + "_" + m
}

var etagRe = regexp.MustCompile(`"[^"]+"`)

type emailConvertRequest struct {
	Params      convertParams
	Input       reqFile
	IfNoneMatch []string
}

func emailConvertDecode(ctx context.Context, r *http.Request) (interface{}, error) {
	r.ParseForm()
	req := emailConvertRequest{Params: convertParams{
		Splitted: r.Form.Get("splitted") == "1",
		OutImg:   r.Form.Get("outimg"),
		ImgSize:  r.Form.Get("imgsize"),
		Merged:   r.Form.Get("merged") == "1" || r.Header.Get("Accept") == "application/pdf",
	}}
	if req.Params.ImgSize == "" {
		req.Params.ImgSize = defaultImageSize
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
	getLogger(ctx).Log("input", req.Input)
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
	Log := logger.Log
	req := request.(emailConvertRequest)
	defer func() { _ = req.Input.Close() }()

	getOutFn := func(params convertParams, hsh string) string {
		return filepath.Join(converter.Workdir,
			fmt.Sprintf("result-%s!%s.zip", hsh, params))
	}

	getCachedFn := func(params convertParams, hsh string) (string, error) {
		if strings.Contains(hsh, "/") {
			return "", fmt.Errorf("bad hsh: %q", hsh)
		}
		outFn := getOutFn(params, hsh)
		outFh, outErr := os.Open(outFn)
		if outErr != nil {
			return outFn, outErr
		}
		defer func() {
			if closeErr := outFh.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}()
		defer func() {
			if err != nil {
				Log("msg", "Removing stale result", "file", outFn)
				_ = os.Remove(outFn)
			}
		}()

		fi, statErr := outFh.Stat()
		if statErr != nil || fi.Size() == 0 {
			return outFn, statErr
		}
		// test correctness of the zip file
		z, zErr := zip.OpenReader(outFh.Name())
		if zErr != nil {
			return outFn, zErr
		}
		_ = z.Close()
		return outFn, nil
	}

	resp := emailConvertResponse{
		r: ctx.Value("http.Request").(*http.Request),
	}

	for _, hsh := range req.IfNoneMatch {
		if _, err = getCachedFn(req.Params, hsh); err == nil {
			resp.NotModified = true
			return resp, nil
		}
	}

	h := sha1.New()
	F := firstN{Data: make([]byte, 0, 1024)}
	inpFn, err := readerToFile(io.TeeReader(req.Input, io.MultiWriter(h, &F)), req.Input.Filename)
	if err != nil {
		return resp, fmt.Errorf("cannot read input file: %v", err)
	}
	req.Params.ContentType = converter.FixContentType(F.Data, req.Params.ContentType, req.Input.Filename)
	Log("msg", "fixed", "params", req.Params)
	if !converter.LeaveTempFiles {
		defer func() { _ = os.Remove(inpFn) }()
	}
	hsh := base64.URLEncoding.EncodeToString(h.Sum(nil))
	if resp.outFn, err = getCachedFn(req.Params, hsh); err == nil {
		err = resp.mergeIfRequested(req.Params, logger)
		return resp, err
	}

	input, err := os.Open(inpFn)
	if err != nil {
		return nil, err
	}

	if !req.Params.Splitted && req.Params.OutImg == "" {
		err = converter.MailToPdfZip(ctx, resp.outFn, input, req.Params.ContentType)
		if err == nil {
			err = resp.mergeIfRequested(req.Params, logger)
		}
	} else {
		err = converter.MailToSplittedPdfZip(ctx, resp.outFn, input, req.Params.ContentType,
			req.Params.Splitted, req.Params.OutImg, req.Params.ImgSize)
	}
	if err != nil {
		Log("msg", "MailToSplittedPdfZip from", "from", input, "out", resp.outFn, "params", req.Params, "error", err)
		return resp, err
	}
	return resp, nil
}

type readSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

type emailConvertResponse struct {
	r           *http.Request
	outFn, hsh  string
	content     readSeekCloser
	NotModified bool
}

func emailConvertEncode(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	resp := response.(emailConvertResponse)
	if resp.NotModified {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}
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
		return nil
	}
	http.ServeFile(w, resp.r, resp.outFn)
	return nil
}

func SaveRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, "http.Request", r)
}

func (resp *emailConvertResponse) mergeIfRequested(params convertParams, logger log.Logger) error {
	if !params.Merged {
		return nil
	}
	// merge PDFs
	tempDir, err := ioutil.TempDir("", "agostle-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	if err = archiver.Unarchive(resp.outFn, tempDir); err != nil {
		return errors.Wrapf(err, "unarchive %q to %q", resp.outFn, tempDir)
	}
	dh, err := os.Open(tempDir)
	if err != nil {
		return errors.Wrap(err, tempDir)
	}
	names, _ := dh.Readdirnames(-1)
	dh.Close()
	mr := pdfMergeRequest{Inputs: make([]reqFile, 0, len(names))}
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
		return errors.Wrapf(err, "merge %v", mr.Inputs)
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
