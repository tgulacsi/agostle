// Copyright 2019 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/h2non/filetype"
	filetypes "github.com/h2non/filetype/types"
)

type H2nonMIMEDetector struct{}
type VasileMIMEDetector struct{}

type MIMEDetector interface {
	Match([]byte) (string, error)
}

var DefaultMIMEDetector = MIMEDetector(MultiMIMEDetector{Detectors: []MIMEDetector{
	FileMIMEDetector{}, HTTPMIMEDetector{}, VasileMIMEDetector{}, H2nonMIMEDetector{},
}})

func MIMEMatch(b []byte) (string, error) { return DefaultMIMEDetector.Match(b) }

func (d H2nonMIMEDetector) Match(b []byte) (string, error) {
	typ, err := filetype.Match(b)
	if typ == filetypes.Unknown {
		return "", err
	}
	return typ.MIME.Type + "/" + typ.MIME.Subtype, err
}
func (d VasileMIMEDetector) Match(b []byte) (string, error) {
	typ := mimetype.Detect(b)
	return typ.String(), nil
}

type HTTPMIMEDetector struct{}

func (d HTTPMIMEDetector) Match(b []byte) (string, error) {
	typ := http.DetectContentType(b)
	if typ == "application/octet-stream" {
		return "", nil
	}
	return typ, nil
}

type FileMIMEDetector struct{}

func (d FileMIMEDetector) Match(b []byte) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := Exec.CommandContext(ctx, "file", "-E", "-b", "--mime-type", "-")
	cmd.Stdin = bytes.NewReader(b)
	b, err := cmd.Output()
	return string(bytes.TrimSpace(b)), err
}

type MultiMIMEDetector struct {
	Detectors []MIMEDetector
	Parallel  bool
}

func (d MultiMIMEDetector) Match(b []byte) (string, error) {
	type result struct {
		Err  error
		Type string
	}
	results := make([]result, len(d.Detectors))
	if !d.Parallel {
		for i, detector := range d.Detectors {
			typ, err := detector.Match(b)
			results[i] = result{Type: typ, Err: err}
		}
	} else {
		var wg sync.WaitGroup
		for i, d := range d.Detectors {
			wg.Add(1)
			i, d := i, d
			go func() {
				typ, err := d.Match(b)
				results[i] = result{Type: typ, Err: err}
				wg.Done()
			}()
		}
		wg.Wait()
	}
	var res string
	var lastErr = errors.New("not found")
	for _, r := range results {
		//fmt.Println(i, r)
		if r.Err != nil {
			if lastErr != nil {
				lastErr = r.Err
			}
			continue
		}
		lastErr = nil
		if res == "" {
			res = r.Type
		}
		continue
	}
	//fmt.Println("result:", res, lastErr)
	return res, lastErr
}
