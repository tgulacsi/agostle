// Copyright 2019 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"errors"
	"net/http"

	"github.com/gabriel-vasile/mimetype"
	"github.com/h2non/filetype"
	filetypes "github.com/h2non/filetype/types"
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
}

func (d MultiMIMEDetector) Match(b []byte) (string, error) {
	var res string
	var lastErr = errors.New("not found")
	for _, detector := range d.Detectors {
		candidate, err := detector.Match(b)
		if err == nil {
			lastErr = nil
			if len(res) < len(candidate) {
				res = candidate
			}
			continue
		}
		if lastErr != nil {
			lastErr = err
		}
	}
	return res, lastErr
}
