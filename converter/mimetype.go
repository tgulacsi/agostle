// Copyright 2019 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/zRedShift/mimemagic"
)

type MagicMIMEDetector struct{}
type VasileMIMEDetector struct{}

type MIMEDetector interface {
	Match([]byte) string
}

var DefaultMIMEDetector = MIMEDetector(MultiMIMEDetector{Detectors: []MIMEDetector{
	FileMIMEDetector{}, HTTPMIMEDetector{}, VasileMIMEDetector{}, MagicMIMEDetector{},
}})

func MIMEMatch(b []byte) string { return DefaultMIMEDetector.Match(b) }

func (d MagicMIMEDetector) Match(b []byte) string {
	return mimemagic.Match(b, "").MediaType()
}
func (d VasileMIMEDetector) Match(b []byte) string {
	return mimetype.Detect(b).String()
}

type HTTPMIMEDetector struct{}

func (d HTTPMIMEDetector) Match(b []byte) string {
	typ := http.DetectContentType(b)
	if typ == "application/octet-stream" {
		return ""
	}
	return typ
}

type FileMIMEDetector struct{}

func (d FileMIMEDetector) Match(b []byte) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := Exec.CommandContext(ctx, "file", "-E", "-b", "--mime-type", "-")
	cmd.Stdin = bytes.NewReader(b)
	b, err := cmd.Output()
	if err != nil {
		logger.Error("FileMIMEDetector", "cmd", cmd.Args, "error", err)
	}
	return string(bytes.TrimSpace(b))
}

type MultiMIMEDetector struct {
	Detectors []MIMEDetector
	Parallel  bool
}

func (d MultiMIMEDetector) Match(b []byte) string {
	results := make([]string, len(d.Detectors))
	if !d.Parallel {
		for i, detector := range d.Detectors {
			results[i] = detector.Match(b)
		}
	} else {
		var wg sync.WaitGroup
		for i, d := range d.Detectors {
			wg.Add(1)
			i, d := i, d
			go func() {
				results[i] = d.Match(b)
				wg.Done()
			}()
		}
		wg.Wait()
	}
	var res string
	for _, r := range results {
		//fmt.Println(i, r)
		if res == "" && r != "application/octet-stream" {
			res = r
		}
		continue
	}
	//fmt.Println("result:", res, lastErr)
	return res
}
