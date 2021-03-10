// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

// Needed: /email/convert?splitted=1&errors=1&id=xxx Accept: images/gif
//  /pdf/merge Accept: application/zip

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
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

	"github.com/UNO-SOFT/otel"
	"github.com/VictoriaMetrics/metrics"
	"github.com/oklog/ulid"
	"github.com/tgulacsi/agostle/converter"
	"github.com/tgulacsi/go/temp"

	"github.com/go-kit/kit/log"
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
				start := time.Now()
				handleFunc.ServeHTTP(w, r)
				mDur.UpdateDuration(start)
			},
		)
	}
	H("/pdf/merge", pdfMergeServer.ServeHTTP)
	H("/email/convert", emailConvertServer.ServeHTTP)
	H("/convert", emailConvertServer.ServeHTTP)
	H("/outlook", outlookToEmailServer.ServeHTTP)
	mux.Handle("/_admin/stop", http.HandlerFunc(adminStopHandler))
	mux.Handle("/", http.DefaultServeMux)

	tp, err := otel.LogTraceProvider(logger.Log)
	if err != nil {
		panic(err)
	}
	otel.SetGlobalTraceProvider(tp)

	return &http.Server{
		Addr:         address,
		ReadTimeout:  300 * time.Second,
		WriteTimeout: 1800 * time.Second,
		Handler:      otel.HTTPMiddleware(otel.GlobalTracer("unosoft.hu/aodb"), &mux),
	}
}

type ctxKey string

const (
	ctxKeyReqID  = ctxKey("reqid")
	ctxKeyCancel = ctxKey("cancel")
	ctxKeyLogger = ctxKey("logger")
)

func SetRequestID(ctx context.Context, name ctxKey) context.Context {
	if name == "" {
		name = ctxKeyReqID
	}
	if ctx.Value(name) != nil {
		return ctx
	}
	return context.WithValue(ctx, name, NewULID().String())
}
func GetRequestID(ctx context.Context, name ctxKey) string {
	if v, ok := ctx.Value(name).(string); ok && v != "" {
		return v
	}
	return NewULID().String()
}

func NewULID() ulid.ULID {
	return ulid.MustNew(ulid.Now(), rand.Reader)
}

var defaultBeforeFuncs = []kithttp.RequestFunc{
	prepareContext,
}

func prepareContext(ctx context.Context, r *http.Request) context.Context {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	ctx = context.WithValue(ctx, ctxKeyCancel, cancel)
	ctx = SetRequestID(ctx, "")
	lgr := getLogger(ctx)
	lgr = log.With(lgr,
		"reqid", GetRequestID(ctx, ""),
		"path", r.URL.Path,
		"method", r.Method,
	)
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		lgr = log.With(lgr, "ip", host)
	}
	ctx = context.WithValue(ctx, ctxKeyLogger, lgr)
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
	Log := log.With(getLogger(ctx), "fn", "dumpRequest").Log
	if err != nil {
		Log("msg", "dumping request", "error", err)
	}
	fn := fmt.Sprintf("%s%06d.dmp", prefix, atomic.AddUint64(&reqSeq, 1))
	if err = ioutil.WriteFile(fn, b, 0660); err != nil {
		Log("msg", "writing", "dumpfile", fn, "error", err)
	} else {
		Log("msg", "Request has been dumped into "+fn)
	}
	return ctx
}

func adminStopHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Refresh", "3;URL=/")
	w.WriteHeader(200)
	fmt.Fprintf(w, `Stopping...`)
	go func() {
		time.Sleep(time.Millisecond * 500)
		logger.Log("msg", "SUICIDE for ask!")
		os.Exit(3)
	}()
}

type reqFile struct {
	io.ReadCloser
	multipart.FileHeader
}

// getOneRequestFile reads the first file from the request (if multipart/),
// or returns the body if not
func getOneRequestFile(ctx context.Context, r *http.Request) (reqFile, error) {
	if r == nil {
		return reqFile{}, errors.New("empty request")
	}
	f := reqFile{ReadCloser: r.Body}
	contentType := r.Header.Get("Content-Type")
	Log := getLogger(ctx).Log
	Log("msg", "readRequestOneFile", "content-type", contentType)
	if !strings.HasPrefix(contentType, "multipart/") {
		f.FileHeader.Header = textproto.MIMEHeader(r.Header)
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Disposition"))
		Log("content-disposition", r.Header.Get("Content-Disposition"), "params", params)
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
	defer r.MultipartForm.RemoveAll()

	for _, fileHeaders := range r.MultipartForm.File {
		for _, fileHeader := range fileHeaders {
			var err error
			if f.ReadCloser, err = fileHeader.Open(); err != nil {
				return f, fmt.Errorf("error opening part %q: %s", fileHeader.Filename, err)
			}
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
	defer r.MultipartForm.RemoveAll()

	files := make([]reqFile, 0, len(r.MultipartForm.File))
	for _, fileHeaders := range r.MultipartForm.File {
		for _, fileHeader := range fileHeaders {
			f := reqFile{FileHeader: *fileHeader}
			if f.ReadCloser, err = fileHeader.Open(); err != nil {
				return nil, fmt.Errorf("error reading part %q: %s", fileHeader.Filename, err)
			}
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return nil, errors.New("no files??")
	}
	return files, nil
}

// readerToFile copies the reader to a temp file and returns its name or error
func readerToFile(r io.Reader, prefix string) (filename string, err error) {
	dfh, e := ioutil.TempFile("", "agostle-"+baseName(prefix)+"-")
	if e != nil {
		err = e
		return
	}
	if sfh, ok := r.(*os.File); ok {
		filename = dfh.Name()
		_ = dfh.Close()
		_ = os.Remove(filename)
		err = temp.LinkOrCopy(sfh.Name(), filename)
		return
	}
	if _, err = io.Copy(dfh, r); err == nil {
		filename = dfh.Name()
	}
	_ = dfh.Close()
	return
}

func tempFilename(prefix string) (filename string, err error) {
	fh, e := ioutil.TempFile("", prefix)
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
		getLogger(ctx).Log("msg", "EMPTY REQUEST")
		return
	}
	getLogger(ctx).Log("msg", "ACCEPT", "method", r.Method, "uri", r.RequestURI, "remote", r.RemoteAddr)
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
func getLogger(ctx context.Context) log.Logger {
	if ctx == nil {
		return logger
	}
	if lgr, ok := ctx.Value(ctxKeyLogger).(log.Logger); ok {
		return lgr
	}
	return logger
}
