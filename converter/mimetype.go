// Copyright 2019 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"net/http"
	"sync"

	"github.com/gabriel-vasile/mimetype"
	"github.com/h2non/filetype"
	filetypes "github.com/h2non/filetype/types"
	errors "golang.org/x/xerrors"
)

type H2nonMIMEDetector struct{}
type VasileMIMEDetector struct{}

type MIMEDetector interface {
	Match([]byte) (string, error)
}

var DefaultMIMEDetector = MIMEDetector(MultiMIMEDetector{Detectors: []MIMEDetector{HTTPMIMEDetector{}, VasileMIMEDetector{}, H2nonMIMEDetector{}}})

func MIMEMatch(b []byte) (string, error) { return DefaultMIMEDetector.Match(b) }

func (d H2nonMIMEDetector) Match(b []byte) (string, error) {
	typ, err := filetype.Match(b)
	if typ == filetypes.Unknown {
		return "", err
	}
	return typ.MIME.Type + "/" + typ.MIME.Subtype, err
}
func (d VasileMIMEDetector) Match(b []byte) (string, error) {
	typ, _ := mimetype.Detect(b)
	return typ, nil
}

type HTTPMIMEDetector struct{}

func (d HTTPMIMEDetector) Match(b []byte) (string, error) {
	typ := http.DetectContentType(b)
	if typ == "application/octet-stream" {
		return "", nil
	}
	return typ, nil
}

type MultiMIMEDetector struct {
	Detectors []MIMEDetector
	Parallel  bool
}

func (d MultiMIMEDetector) Match(b []byte) (string, error) {
	type result struct {
		Type string
		Err  error
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
		if r.Err != nil {
			if lastErr != nil {
				lastErr = r.Err
			}
			continue
		}
		lastErr = nil
		if len(res) < len(r.Type) {
			res = r.Type
		}
		continue
	}
	return res, lastErr
}
