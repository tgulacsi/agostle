// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"

	"context"

	"github.com/tgulacsi/agostle/converter"

	"github.com/go-kit/kit/log"
	kithttp "github.com/go-kit/kit/transport/http"
)

var pdfMergeServer = kithttp.NewServer(
	pdfMergeEP,
	pdfMergeDecode,
	pdfMergeEncode,
	kithttp.ServerBefore(defaultBeforeFuncs...),
	kithttp.ServerAfter(kithttp.SetContentType("application/pdf")),
)

func pdfMergeDecode(ctx context.Context, r *http.Request) (interface{}, error) {
	Log := log.With(logger, "fn", "pdfMergeDecode").Log
	inputs, err := getRequestFiles(r)
	if err != nil {
		Log("msg", "getRequestFiles", "error", err)
		return nil, err
	}
	req := pdfMergeRequest{Inputs: inputs}
	switch r.URL.Query().Get("sort") {
	case "0":
		req.Sort = NoSort
	case "1":
		req.Sort = DoSort
	default:
		req.Sort = DefaultSort
	}
	return req, nil
}

func pdfMergeEP(ctx context.Context, request interface{}) (response interface{}, err error) {
	req, ok := request.(pdfMergeRequest)
	if !ok {
		return nil, fmt.Errorf("awaited pdfMergeRequest, got %T", request)
	}
	defer func() {
		for _, f := range req.Inputs {
			_ = f.Close()
		}
	}()

	Log := log.With(logger, "fn", "pdfMergeEP").Log
	if sortBeforeMerge && req.Sort != NoSort || !sortBeforeMerge && req.Sort != DoSort {
		Log("msg", "sorting filenames, as requested", "ask", req.Sort, "config", sortBeforeMerge)
		sort.Sort(ByName(req.Inputs))
	}

	filenames := make([]string, len(req.Inputs))
	if !converter.LeaveTempFiles {
		defer func() {
			for _, fn := range filenames {
				if fn != "" {
					_ = os.Remove(fn)
				}
			}
		}()
	}
	for i, f := range req.Inputs {
		if filenames[i], err = readerToFile(f.ReadCloser, f.Filename); err != nil {
			Log("msg", "readerToFile", "file", f.Filename, "error", err)
			return nil, fmt.Errorf("error saving %q: %s", f.Filename, err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	dst, err := tempFilename("pdfmerge-")
	if err != nil {
		Log("msg", "tempFilename", "error", err)
		return nil, err
	}
	defer os.Remove(dst)
	Log("msg", "PdfMerge", "dst", dst, "filenames", filenames)
	if err = converter.PdfMerge(ctx, dst, filenames...); err != nil {
		Log("msg", "PdfMerge", "dst", dst, "filenames", filenames, "error", err)
		return nil, err
	}
	f, err := os.Open(dst)
	if err != nil {
		Log("msg", "Open(dst)", "dst", dst, "error", err)
		return nil, err
	}
	return f, nil
}

func pdfMergeEncode(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	Log := log.With(logger, "fn", "pdfMergeEncode").Log
	if f, ok := response.(interface {
		Stat() (os.FileInfo, error)
	}); ok {
		if fi, err := f.Stat(); err != nil {
			Log("msg", "response file Stat", "error", err)
		} else {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
		}
	} else {
		Log("msg", "pdfMergeEncode non-statable response!", "response", fmt.Sprintf("%#v %T", response, response))
	}
	dst := response.(io.ReadCloser)
	defer func() { _ = dst.Close() }()
	// successful PdfMerge recreated the dest file
	_, err := io.Copy(w, dst)
	Log("msg", "read", "file", dst, "error", err)
	return err
}

type pdfMergeRequest struct {
	Inputs []reqFile
	Sort   sortMode
}

type sortMode uint8

const (
	DefaultSort = sortMode(iota)
	NoSort
	DoSort
)

type ByName []reqFile

func (b ByName) Len() int           { return len(b) }
func (b ByName) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByName) Less(i, j int) bool { return b[i].Filename < b[j].Filename }
