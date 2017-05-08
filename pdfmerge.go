// Copyright 2013 The Agostle Authors. All rights reserved.
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

	"github.com/pkg/errors"
)

var pdfMergeServer = kithttp.NewServer(
	pdfMergeEP,
	pdfMergeDecode,
	pdfMergeEncode,
	kithttp.ServerBefore(defaultBeforeFuncs...),
	kithttp.ServerAfter(kithttp.SetContentType("application/pdf")),
)

func pdfMergeDecode(ctx context.Context, r *http.Request) (interface{}, error) {
	inputs, err := getRequestFiles(r)
	if err != nil {
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
		return nil, errors.New(fmt.Sprintf("awaited pdfMergeRequest, got %T", request))
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
		return nil, err
	}
	if err := converter.PdfMerge(ctx, dst, filenames...); err != nil {
		Log("msg", "PdfMerge", "dst", dst, "filenames", filenames, "error", err)
		return nil, err
	}
	f, err := os.Open(dst)
	if err != nil {
		return nil, err
	}
	_ = os.Remove(dst)
	return f, nil
}

func pdfMergeEncode(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	Log := logger.Log
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
	return err
}

type pdfMergeRequest struct {
	Sort   sortMode
	Inputs []reqFile
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
