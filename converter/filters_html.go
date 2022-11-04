// Copyright 2017, 2020 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"context"

	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"

	"github.com/tgulacsi/go/i18nmail"
)

type cidGroup struct {
	cids   map[string]string
	htmlFn string
}

// HTMLPartFilter reads multipart/alternative (text/plain + text/html), preferring the html
// part + groups the multipart/related images which are referred in the html.
//
// multipart/related encapsulates multipart/alternative,
// which contains text/plain and text/html, the related part contains images,
// too - at least usually.
func HTMLPartFilter(ctx context.Context,
	inch <-chan i18nmail.MailPart, outch chan<- i18nmail.MailPart,
	files chan<- ArchFileItem, errch chan<- error,
) {
	logger := getLogger(ctx)
	defer func() {
		close(outch)
	}()
	ctx, wd := prepareContext(ctx, "")

	var (
		alter      = ""
		converter  = GetConverter(textHtml, nil)
		aConverter Converter
		// nosemgrep
		tbd = make(map[string]struct{}, 4)
	)
	if !LeaveTempFiles {
		defer func() {
			tbdA := make([]string, 0, len(tbd))
			for k := range tbd {
				tbdA = append(tbdA, k)
			}
			sort.Strings(tbdA)
			for i := len(tbdA) - 1; i > -1; i-- {
				_ = unlinkAll(tbdA[i])
			}
		}()
	}

	html2pdf := func(fn string) (string, error) {
		if fn == "" {
			logger.Info("empty filename!!!")
			return "", errors.New("empty filename")
		}
		tbd[fn] = struct{}{}
		destfn := filepath.Join(wd, filepath.Base(fn)+".pdf")
		fh, err := os.Open(fn)
		if err != nil {
			err = fmt.Errorf("open html %s: %w", fn, err)
		} else {
			err = converter(ctx, destfn, fh, textHtml)
			fh.Close()
			if err == nil {
				return destfn, nil
			}
			err = fmt.Errorf("converting %s to %s: %w", fn, destfn, err)
		}
		logger.Info("html2pdf", "error", err)
		// nosemgrep: go.lang.correctness.useless-eqeq.eqeq-is-bad
		if alter != "" && aConverter != nil {
			logger.Info("html2pdf using alternative content " + alter)
			if fh, err = os.Open(alter); err != nil {
				err = fmt.Errorf("open txt %s: %w", alter, err)
			} else {
				err = aConverter(ctx, destfn, fh, textPlain)
				fh.Close()
				if err != nil {
					err = fmt.Errorf("converting %s to %s: %w", alter, destfn, err)
				}
			}
			alter, aConverter = "", nil
		}
		return destfn, err
	}

	groups := make(map[int][]cidGroup, 4)
	var (
		this, last      int
		err             error
		ok              bool
		fn, dn, cid     string
		parent, grandpa *i18nmail.MailPart
	)
	seen := make(map[string]struct{})
	last = -1
	for part := range inch {

		this = -1
		parent = part.Parent
		if parent == nil {
			grandpa = nil
		} else {
			grandpa = parent.Parent
		}
		logger.Info("part", "seq", part.Seq, "ct", part.ContentType, "groups", groups)
		if part.ContentType == textPlain || part.ContentType == textHtml {
			//if part.Parent.ContentType != "multipart/alternative" || part.Parent.ContentType != "multipart/related" {
			if part.ContentType == textPlain && part.Parent != nil && part.Parent.ContentType != "multipart/alternative" {
				goto Skip
			}
			if grandpa != nil {
				this = grandpa.Seq
			}
			if this != last && last > -1 && len(groups[last]) > 0 {
				for _, cg := range groups[last] {
					if fn, err = html2pdf(cg.htmlFn); err != nil {
						goto Error
					}
					files <- ArchFileItem{Filename: fn}
				}
			}

			if grandpa != nil {
				dn = filepath.Join(wd,
					fmt.Sprintf("%02d#%03d.text--html", grandpa.Level, grandpa.Seq))
			} else {
				dn = wd
			}
			// nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission
			_ = os.Mkdir(dn, 0755) //ignore errors
			fn = ""
			if parent != nil {
				fn = fmt.Sprintf("%02d#%03d.index", parent.Level, parent.Seq)
				if _, ok := seen[fn]; ok {
					fn = ""
				} else {
					seen[fn] = struct{}{}
				}
			}
			if fn == "" {
				fn = fmt.Sprintf("%02d#%03d.index", part.Level, part.Seq)
			}

			var cids map[string]string
			if part.ContentType == textPlain {
				fn = fn + ".txt"
			} else {
				body := fixXMLHeader(part.GetBody())
				cids = make(map[string]string, 4)
				body = NewCidMapper(cids, "images", body)
				entity, _ := i18nmail.NewEntity(part.Header, body)
				part, _ = part.WithEntity(entity)
				fn = fmt.Sprintf("%s-%02d.html", fn, this)
			}
			fn = filepath.Join(dn, fn)
			//_, _ = part.Body.Seek(0, 0)
			if err = writeToFile(ctx, fn, part.GetBody(), part.ContentType /*, mailHeader*/); err != nil {
				goto Error
			}
			tbd[fn] = struct{}{}
			if part.ContentType == textPlain {
				alter = fn
				aConverter = GetConverter(textPlain, part.MediaType)
			} else if part.ContentType == textHtml {
				//log.Printf("last==this? %b  cidmap: %s", last == this, cids)
				groups[this] = append(groups[this], cidGroup{htmlFn: fn, cids: cids})
			}
			last = this
			continue
		}
		if !strings.HasPrefix(part.ContentType, "image/") {
			goto Skip
		}
		cid = strings.Trim(part.Header.Get("Content-ID"), "<>")
		if cid == "" {
			goto Skip
		}
		this = parent.Seq

		{
			found := false
			if len(groups) > 0 {
				for act := &part; act != nil; act = act.Parent {
					if _, ok = groups[act.Seq]; ok {
						found = true
						this = act.Seq
						break
					}
				}
			}
			if !found {
				err = fmt.Errorf("WARN this=%d not in %v", part.Seq, groups)
				logger.Info("SKIP not found", "error", err)
				goto Skip
			}
		}

		// K-MT9641 skip images which aren't in the .html
		for _, cg := range groups[this] {
			if fn, ok = cg.cids[cid]; !ok {
				goto Skip
			}
			fn = filepath.Join(filepath.Dir(cg.htmlFn), fn)

			// nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission
			_ = os.Mkdir(filepath.Dir(fn), 0755) // ignore error
			logger.Info("save", "file", fn, "cid", cid, "htmlFn", cg.htmlFn)
			//_, _ = part.Body.Seek(0, 0)
			if err = writeToFile(ctx, fn, part.GetBody(), part.ContentType /*, mailHeader*/); err != nil {
				goto Error
			}
			tbd[filepath.Dir(fn)] = struct{}{}
		}

		continue
	Error:
		logger.Error(err, "HTMLPartFilter")
		if err != nil {
			errch <- err
		}
	Skip:
		outch <- part
	}

	for _, vv := range groups {
		for _, v := range vv {
			if fn, err = html2pdf(v.htmlFn); err != nil {
				errch <- err
			}
			files <- ArchFileItem{Filename: fn}
		}
	}
}

// fixXMLHeader fixes <?xml version="1.0" encoding=...?> to <?xml version="1.0" charset=...?>
func fixXMLHeader(r io.Reader) io.Reader {
	b := make([]byte, 64)
	n, _ := io.ReadAtLeast(r, b, 32)
	b = b[:n]
	bad := []byte(`<?xml version="1.0" encoding=`)
	if i := bytes.Index(b, bad); i >= 0 {
		good := []byte(`<?xml version="1.0" charset=`)
		copy(b[i:], good)
		copy(b[i+len(good):], b[i+len(bad):])
		if i = len(bad) - len(good); i > 0 {
			b = b[:len(b)-i]
		}
	}
	return io.MultiReader(bytes.NewReader(b), r)
}

// fixXMLCharset fixes the charset to utf-8, and converts such
func fixXMLCharset(ctx context.Context, r io.Reader) io.Reader {
	b := make([]byte, 64)
	n, _ := io.ReadAtLeast(r, b, 32)
	b = b[:n]
	ori := io.MultiReader(bytes.NewReader(b), r)

	if i := bytes.Index(b, []byte("?>")); i >= 0 {
		if j := bytes.LastIndex(b[:i], []byte("charset=")); j >= 0 {
			cs := string(bytes.Trim(bytes.TrimSpace(b[j+8:i]), `'"`))
			enc, err := htmlindex.Get(cs)
			if err != nil {
				getLogger(ctx).Info("get decoder for", "charset", cs, "error", err)
				return ori
			}
			return io.MultiReader(bytes.NewReader(b[:j]), bytes.NewReader(b[i:]),
				transform.NewReader(r, enc.NewDecoder()))
		}
	}
	return ori
}

// SaveOriHTMLFilter reads text/html and saves it.
func SaveOriHTMLFilter(ctx context.Context,
	inch <-chan i18nmail.MailPart, outch chan<- i18nmail.MailPart,
	files chan<- ArchFileItem, errch chan<- error,
) {
	logger := getLogger(ctx)
	defer func() {
		close(outch)
	}()
	_, wd := prepareContext(ctx, "")

	if !SaveOriginalHTML {
		for part := range inch {
			outch <- part
		}
		return
	}

	for part := range inch {
		if part.ContentType == textHtml {
			fn := filepath.Join(wd, fmt.Sprintf("%02d#%03d.ori.html", part.Level, part.Seq))
			logger.Info("saving original html " + fn)
			if orifh, e := os.Create(fn); e == nil {
				//_, _ = part.Body.Seek(0, 0)
				if _, e = io.Copy(orifh, part.GetBody()); e != nil {
					logger.Info("write ori to", "dest", orifh.Name(), "error", e)
				}
				orifh.Close()
				if fh, err := os.Open(orifh.Name()); e != nil {
					logger.Info("reopen", "file", orifh.Name(), "error", err)
					errch <- err
					continue
				} else if part, err = part.Spawn().WithBody(fh); err != nil {
					errch <- err
					continue
				}

			}
		}
		outch <- part
	}
}
