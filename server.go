// Copyright 2017, 2024 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

// Needed: /email/convert?splitted=1&errors=1&id=xxx Accept: images/gif
//  /pdf/merge Accept: application/zip

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httputil"
	_ "net/http/pprof"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/UNO-SOFT/zlog/v2"
	"github.com/UNO-SOFT/zlog/v2/slog"
	"github.com/VictoriaMetrics/metrics"
	"github.com/google/renameio"
	"github.com/tgulacsi/agostle/converter"

	kithttp "github.com/go-kit/kit/transport/http"
)

var (
	defaultImageSize = "640x640"
	self             = ""
	sortBeforeMerge  = false
)

// newHTTPServer returns a new, stoppable HTTP server
func newHTTPServer(address string, saveReq bool) *http.Server {
	onceOnStart.Do(onStart)

	if saveReq {
		defaultBeforeFuncs = append(defaultBeforeFuncs, dumpRequest)
	}

	var mux http.ServeMux
	s := &http.Server{
		Addr:         address,
		ReadTimeout:  300 * time.Second,
		WriteTimeout: 1800 * time.Second,
		Handler:      &mux,
	}

	//mux.Handle("/debug/pprof", pprof.Handler)
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) { metrics.WritePrometheus(w, true) })

	H := func(path string, handleFunc http.HandlerFunc) {
		mName := fmt.Sprintf("request_duration_seconds{method=%%q,handler=%q}", strings.Replace(path[1:], "/", "_", -1))
		mGet := metrics.GetOrCreateHistogram(fmt.Sprintf(mName, "GET"))
		mPost := metrics.GetOrCreateHistogram(fmt.Sprintf(mName, "POST"))
		mux.HandleFunc(
			path,
			func(w http.ResponseWriter, r *http.Request) {
				var mDur *metrics.Histogram
				switch r.Method {
				case "GET":
					mDur = mGet
				case "POST":
					mDur = mPost
				default:
					mDur = metrics.GetOrCreateHistogram(fmt.Sprintf(mName, r.Method))
				}
				ctx := r.Context()
				reqID := converter.GetRequestID(ctx)
				ctx = converter.SetRequestID(ctx, reqID)
				ctx, wd := converter.PrepareContext(ctx, reqID)
				r = r.WithContext(ctx)
				start := time.Now()
				handleFunc.ServeHTTP(w, r)
				mDur.UpdateDuration(start)
				os.RemoveAll(wd)
			},
		)
	}
	H("/pdf/merge", pdfMergeServer.ServeHTTP)
	H("/email/convert", emailConvertServer.ServeHTTP)
	H("/convert", emailConvertServer.ServeHTTP)
	H("/outlook", outlookToEmailServer.ServeHTTP)
	//H("/stem", stemServer.ServeHTTP)
	mux.Handle("/_admin/stop", mkAdminStopHandler(s))
	mux.Handle("/", http.DefaultServeMux)

	return s
}

type ctxCancel struct{}

func SetRequestCancel(ctx context.Context, cancel context.CancelFunc) context.Context {
	return context.WithValue(ctx, ctxCancel{}, cancel)
}
func SetRequestID(ctx context.Context, reqID string) context.Context {
	return converter.SetRequestID(ctx, reqID)
}
func GetRequestID(ctx context.Context) string {
	return converter.GetRequestID(ctx)
}

func NewULID() string { return converter.NewULID().String() }

var defaultBeforeFuncs = []kithttp.RequestFunc{
	prepareContext,
}

func prepareContext(ctx context.Context, r *http.Request) context.Context {
	// nosemgrep: dgryski.semgrep-go.contextcancelable.cancelable-context-not-systematically-cancelled
	ctx, cancel := context.WithTimeout(ctx, *converter.ConfChildTimeout)
	ctx = SetRequestCancel(ctx, cancel)
	ctx = SetRequestID(ctx, "")
	lgr := getLogger(ctx)
	lgr = lgr.With(
		slog.String("reqID", GetRequestID(ctx)),
		slog.String("path", r.URL.Path),
		slog.String("method", r.Method),
	)
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		lgr = lgr.With(slog.String("ip", host))
	}
	ctx = zlog.NewSContext(ctx, lgr)
	logAccept(ctx, r)
	return ctx
}

func dumpRequest(ctx context.Context, req *http.Request) context.Context {
	if req == nil {
		return ctx
	}
	prefix := filepath.Join(converter.Workdir, time.Now().Format("20060102_150405")+"-")
	var reqSeq uint64
	b, err := httputil.DumpRequest(req, true)
	logger := getLogger(ctx).With("fn", "dumpRequest")
	if err != nil {
		logger.Error("dumping request", "error", err)
	}
	fn := fmt.Sprintf("%s%06d.dmp", prefix, atomic.AddUint64(&reqSeq, 1))
	if err = os.WriteFile(fn, b, 0660); err != nil {
		logger.Error("writing", "dumpfile", fn, "error", err)
	} else {
		logger.Info("Request has been dumped into " + fn)
	}
	return ctx
}

func mkAdminStopHandler(s interface{ Shutdown(context.Context) error }) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Refresh", "3;URL=/")
		w.WriteHeader(200)
		logger.Info("/_admin/stop", "header", r.Header, "from", r.RemoteAddr)
		fmt.Fprintf(w, `Stopping...`)
		_ = s.Shutdown(r.Context())
		go func() {
			time.Sleep(time.Millisecond * 500)
			logger.Info("SUICIDE for ask!")
			os.Exit(3)
		}()
	})
}

type reqFile struct {
	io.ReadCloser
	multipart.FileHeader
}

func (f reqFile) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.FileHeader)
}
func (f reqFile) String() string {
	b, err := f.MarshalJSON()
	s := string(b)
	if err != nil {
		s += "\n" + err.Error()
	}
	return s
}

// getOneRequestFile reads the first file from the request (if multipart/),
// or returns the body if not
func getOneRequestFile(ctx context.Context, r *http.Request) (reqFile, error) {
	if r == nil {
		return reqFile{}, errors.New("empty request")
	}
	f := reqFile{ReadCloser: r.Body}
	contentType := r.Header.Get("Content-Type")
	logger := getLogger(ctx)
	logger.Info("readRequestOneFile", "content-type", contentType)
	if !strings.HasPrefix(contentType, "multipart/") {
		f.FileHeader.Header = textproto.MIMEHeader(r.Header)
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Disposition"))
		logger.Info("getOneRequestFile", "content-disposition", r.Header.Get("Content-Disposition"), "params", params)
		f.FileHeader.Filename = params["filename"]
		return f, nil
	}
	defer r.Body.Close()
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return f, fmt.Errorf("error parsing request as multipart-form: %w", err)
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File) == 0 {
		return f, errors.New("no files?")
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	for _, fileHeaders := range r.MultipartForm.File {
		for _, fileHeader := range fileHeaders {
			rc, err := fileHeader.Open()
			if err != nil {
				return f, fmt.Errorf("error opening part %q: %w", fileHeader.Filename, err)
			}
			f.ReadCloser = rc
			if fileHeader != nil {
				f.FileHeader = *fileHeader
				return f, nil
			}
		}
	}
	return f, nil
}

// getRequestFiles reads the files from the request, and calls readerToFile on them
func getRequestFiles(r *http.Request) ([]reqFile, error) {
	if r.Body != nil {
		defer func() { _ = r.Body.Close() }()
	}
	err := r.ParseMultipartForm(1 << 20)
	if err != nil {
		return nil, fmt.Errorf("cannot parse request as multipart-form: %w", err)
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File) == 0 {
		return nil, errors.New("no files?")
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	files := make([]reqFile, 0, len(r.MultipartForm.File))
	for _, fileHeaders := range r.MultipartForm.File {
		for _, fileHeader := range fileHeaders {
			rc, err := fileHeader.Open()
			if err != nil {
				return nil, fmt.Errorf("error reading part %q: %w", fileHeader.Filename, err)
			}
			f := reqFile{ReadCloser: rc, FileHeader: *fileHeader}
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return nil, errors.New("no files??")
	}
	return files, nil
}

type pendingFile interface {
	Name() string
	io.ReadWriteSeeker
	io.Closer
	Cleanup() error
	CloseAtomicallyReplace() error
}

// readerToFile copies the reader to a temp file and returns its name or error
func readerToFile(r io.Reader, prefix string) (pendingFile, error) {
	pat := "agostle-" + baseName(prefix) + "-"
	dfh, err := renameio.TempFile("", pat)
	if err != nil {
		return nil, fmt.Errorf("renameio.TempFile: %w", err)
	}
	var buf bytes.Buffer
	if _, err = io.Copy(dfh, io.TeeReader(r, &buf)); err != nil {
		logger.Error("readerToFile renameio.TempFile", "error", err)
		_ = dfh.Cleanup()
		if dfh, err := os.CreateTemp("", pat); err == nil {
			if _, err = io.Copy(dfh, io.MultiReader(bytes.NewReader(buf.Bytes()), r)); err == nil {
				if _, err = dfh.Seek(0, 0); err == nil {
					return dummyPendingFile{File: dfh}, nil
				}
			}
		}
		return nil, fmt.Errorf("copy from %v to %v: %w", r, dfh, err)
	}
	_, err = dfh.Seek(0, 0)
	return dfh, err
}

type dummyPendingFile struct {
	*os.File
}

func (f dummyPendingFile) CloseAtomicallyReplace() error { return f.File.Close() }
func (f dummyPendingFile) Cleanup() error {
	err := f.File.Close()
	_ = os.Remove(f.File.Name())
	return err
}

func tempFilename(prefix string) (filename string, err error) {
	fh, e := os.CreateTemp("", prefix)
	if e != nil {
		err = e
		return
	}
	filename = fh.Name()
	_ = fh.Close()
	return
}

func logAccept(ctx context.Context, r *http.Request) {
	if r == nil {
		getLogger(ctx).Info("EMPTY REQUEST")
		return
	}
	getLogger(ctx).Info("ACCEPT", "method", r.Method, "uri", r.RequestURI, "remote", r.RemoteAddr)
}

func baseName(fileName string) string {
	if fileName == "" {
		return ""
	}
	i := strings.LastIndexAny(fileName, "/\\")
	if i >= 0 {
		fileName = fileName[i+1:]
	}
	return fileName
}
func getLogger(ctx context.Context) *slog.Logger {
	if lgr := zlog.SFromContext(ctx); lgr != nil {
		return lgr
	}
	return logger
}
