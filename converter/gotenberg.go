// Copyright 2023 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: Apache-2.0

package converter

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/renameio/v2"
)

type Gotenberg struct {
	URL    string
	mu     sync.RWMutex
	Client *http.Client
	url    *url.URL
}

func (g *Gotenberg) Valid() bool {
	g.mu.RLock()
	ok := g.url != nil
	disabled := g.URL == ""
	g.mu.RUnlock()
	if ok {
		return true
	}
	if disabled {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.url != nil {
		return true
	}
	var err error
	if g.url, err = url.Parse(g.URL); err != nil {
		return false
	}
	if g.Client == nil {
		g.Client = http.DefaultClient
	}
	return true
}

func (g *Gotenberg) PostFileNames(ctx context.Context, destfn string, urlPath string, filenames []string, contentType string) error {
	if !g.Valid() {
		return fmt.Errorf("disabled")
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	pr, pw := io.Pipe()
	bw := bufio.NewWriter(pw)
	mw := multipart.NewWriter(bw)
	go func() {
		defer pw.Close()
		var exts []string
		if contentType != "" {
			exts, _ = mime.ExtensionsByType(contentType)
		}
		for _, fn := range filenames {
			if err := func() error {
				bfn := nakeFilename(fn)
				var hasExt bool
				for _, ext := range exts {
					if hasExt = strings.HasSuffix(bfn, ext); hasExt {
						break
					}
				}
				if !hasExt {
					bfn += exts[0]
				}
				logger.Debug("gotenbert", "file", fn, "base", bfn)
				part, err := mw.CreateFormFile("files", bfn)
				if err != nil {
					return err
				}
				fh, err := os.Open(fn)
				if err != nil {
					return fmt.Errorf("%s: %w", fn, err)
				}
				defer fh.Close()
				_, err = io.Copy(part, fh)
				fh.Close()
				if err != nil {
					return fmt.Errorf("read %s: %w", fn, err)
				}
				return nil
			}(); err != nil {
				pw.CloseWithError(err)
				return
			}
		}
		if err := mw.Close(); err != nil {
			pw.CloseWithError(err)
			return
		}
		pw.CloseWithError(bw.Flush())
	}()

	req, err := http.NewRequestWithContext(ctx, "POST", g.url.JoinPath(urlPath).String(), pr)
	if err != nil {
		return err
	}
	if reqID := GetRequestID(ctx); reqID != "" {
		req.Header.Set("Gotenberg-Trace", reqID)
	}
	req.Header.Set("Gotenberg-Output-Filename", destfn)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	client := g.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 || resp.Body == nil {
		return fmt.Errorf("%s: %s", req.URL, resp.Status)
	}

	fh, err := renameio.NewPendingFile(destfn, renameio.WithTempDir(filepath.Dir(destfn)))
	if err != nil {
		return err
	}
	defer fh.Cleanup()
	if _, err := io.Copy(fh, resp.Body); err != nil {
		return err
	}
	return fh.CloseAtomicallyReplace()
}
