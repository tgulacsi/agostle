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
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"context"

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
	Splitted                     bool
}

func (p convertParams) String() string {
	c := "m"
	if p.Splitted {
		c = "s"
	}
	return strings.Replace(p.ContentType, "/", "--", -1) + "_" + strings.Replace(p.OutImg, "/", "--", -1) + "_" + p.ImgSize + "_" + c
}

var etagRe = regexp.MustCompile(`"[^"]+"`)

type emailConvertRequest struct {
	Params      convertParams
	Input       reqFile
	IfNoneMatch []string
}

func emailConvertDecode(ctx context.Context, r *http.Request) (interface{}, error) {
	req := emailConvertRequest{Params: convertParams{
		Splitted: r.Form.Get("splitted") == "1",
		OutImg:   r.Form.Get("outimg"),
		ImgSize:  r.Form.Get("imgsize"),
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
	contentType := req.Input.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = "message/rfc822"
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
	Log := getLogger(ctx).Log
	req := request.(emailConvertRequest)
	defer func() { _ = req.Input.Close() }()

	getOutFn := func(hsh string) string {
		return filepath.Join(converter.Workdir,
			fmt.Sprintf("result-%s!%s.zip", hsh, req.Params))
	}

	getCachedFn := func(hsh string) (string, error) {
		if strings.Contains(hsh, "/") {
			return "", fmt.Errorf("bad hsh: %q", hsh)
		}
		outFn := getOutFn(hsh)
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
		if _, err = getCachedFn(hsh); err == nil {
			resp.NotModified = true
			return resp, nil
		}
	}

	h := sha1.New()
	inpFn, err := readerToFile(io.TeeReader(req.Input, h), req.Input.Filename)
	if err != nil {
		return resp, fmt.Errorf("cannot read input file: %v", err)
	}
	if !converter.LeaveTempFiles {
		defer func() { _ = os.Remove(inpFn) }()
	}
	hsh := base64.URLEncoding.EncodeToString(h.Sum(nil))
	if resp.outFn, err = getCachedFn(hsh); err == nil {
		return resp, nil
	}

	input, err := os.Open(inpFn)
	if err != nil {
		return nil, err
	}

	if !req.Params.Splitted && req.Params.OutImg == "" {
		err = converter.MailToPdfZip(ctx, resp.outFn, input, req.Params.ContentType)
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

type emailConvertResponse struct {
	r           *http.Request
	outFn, hsh  string
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
	http.ServeFile(w, resp.r, resp.outFn)
	return nil
}

func SaveRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, "http.Request", r)
}
