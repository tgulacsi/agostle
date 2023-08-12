// Copyright 2017, 2022 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"

	"github.com/tgulacsi/agostle/converter"

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
	logger := logger.With(slog.String("fn", "pdfMergeDecode"))
	inputs, err := getRequestFiles(r)
	if err != nil {
		logger.Error("getRequestFiles", "error", err)
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

	logger := logger.With("fn", "pdfMergeEP")
	if sortBeforeMerge && req.Sort != NoSort || !sortBeforeMerge && req.Sort != DoSort {
		logger.Info("sorting filenames, as requested", "ask", req.Sort, "config", sortBeforeMerge)
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
	tbd := make([]func() error, 0, len(req.Inputs))
	defer func() {
		for _, f := range tbd {
			_ = f()
		}
	}()
	for i, f := range req.Inputs {
		tfh, err := readerToFile(f.ReadCloser, f.Filename)
		if err != nil {
			logger.Error("readerToFile", "file", f.Filename, "error", err)
			return nil, fmt.Errorf("error saving %q: %w", f.Filename, err)
		}
		tbd = append(tbd, tfh.Cleanup)
		filenames[i] = tfh.Name()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	dst, err := tempFilename("pdfmerge-")
	if err != nil {
		logger.Error("tempFilename", "error", err)
		return nil, err
	}
	defer os.Remove(dst)
	logger.Info("PdfMerge", "dst", dst, "filenames", filenames)
	if err = converter.PdfMerge(ctx, dst, filenames...); err != nil {
		logger.Error("PdfMerge", "dst", dst, "filenames", filenames, "error", err)
		return nil, err
	}
	f, err := os.Open(dst)
	if err != nil {
		logger.Error("Open(dst)", "dst", dst, "error", err)
		return nil, err
	}
	return f, nil
}

func pdfMergeEncode(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	logger := logger.With("fn", "pdfMergeEncode")
	if f, ok := response.(interface {
		Stat() (os.FileInfo, error)
	}); ok {
		if fi, err := f.Stat(); err != nil {
			logger.Error("response file Stat", "error", err)
		} else {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
		}
	} else {
		logger.Info("pdfMergeEncode non-statable response!", "response", fmt.Sprintf("%#v %T", response, response))
	}
	dst := response.(io.ReadCloser)
	defer func() { _ = dst.Close() }()
	// successful PdfMerge recreated the dest file
	_, err := io.Copy(w, dst)
	logger.Info("read", "file", dst, "error", err)
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
