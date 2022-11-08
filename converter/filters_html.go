// Copyright 2017, 2022 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bufio"
	"bytes"
	"encoding/base64"
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

	type withAlternate struct {
		body            io.Reader
		alter, fileName string
		aConverter      Converter
	}

	var (
		alter      = ""
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

	cids := make(map[string]*usedPart)
	var htmlParts []withAlternate
	var (
		err             error
		parent, grandpa *i18nmail.MailPart
	)
	seen := make(map[string]struct{})
	this := -1
	for part := range inch {
		part := part
		parent = part.Parent
		if parent == nil {
			grandpa = nil
		} else {
			grandpa = parent.Parent
		}
		logger.Info("part", "seq", part.Seq, "ct", part.ContentType)
		if part.ContentType == textPlain || part.ContentType == textHtml {
			//if part.Parent.ContentType != "multipart/alternative" || part.Parent.ContentType != "multipart/related" {
			if part.ContentType == textPlain && part.Parent != nil && part.Parent.ContentType != "multipart/alternative" {
				goto Skip
			}
			if grandpa != nil {
				this = grandpa.Seq
			}

			dn := wd
			if grandpa != nil {
				dn = filepath.Join(wd,
					fmt.Sprintf("%02d#%03d.text--html", grandpa.Level, grandpa.Seq))
				_ = os.Mkdir(dn, 0755) //ignore errors
				tbd[dn] = struct{}{}
			}
			// nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission
			var fn string
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
			fn = filepath.Join(dn, fn)
			if part.ContentType == textHtml {
				fn += fmt.Sprintf("-%02d.html", this)
				htmlParts = append(htmlParts, withAlternate{
					body:     part.GetBody(),
					fileName: fn,
					alter:    alter, aConverter: aConverter,
				})
				alter, aConverter = "", nil
			} else {
				fn += ".txt"
				//_, _ = part.Body.Seek(0, 0)
				if err = writeToFile(ctx, fn, part.GetBody() /*part.ContentType , mailHeader*/); err != nil {
					goto Error
				}
				tbd[fn] = struct{}{}
				alter = fn
				aConverter = GetConverter(textPlain, part.MediaType)
			}
			continue
		} else if !strings.HasPrefix(part.ContentType, "image/") {
			goto Skip
		} else if cid := strings.Trim(part.Header.Get("Content-ID"), "<>"); cid == "" {
			goto Skip
		} else {
			cids[cid] = &usedPart{MailPart: &part}
		}

		this = parent.Seq

		continue
	Error:
		if err != nil {
			logger.Error(err, "HTMLPartFilter")
			if err != nil {
				errch <- err
			}
		}
	Skip:
		outch <- part
	}

	logger.Info("write htmlParts", "htmlParts", len(htmlParts))
	for _, part := range htmlParts {
		body := fixXMLHeader(part.body)
		//body = NewCidMapper(cids, "images", body)
		if body, err = embedCids(body, cids); err != nil {
			logger.Error(err, "embedCids")
		}
		fn := part.fileName
		_ = os.MkdirAll(filepath.Dir(fn), 0700)
		if err = writeToFile(ctx, fn, body /*"text/html" , mailHeader*/); err != nil {
			logger.Error(err, "writeToFile", "file", fn)
			errch <- err
			continue
		}
		tbd[fn] = struct{}{}
		if fn, err = html2pdf(ctx, fn, part.alter, part.aConverter); err != nil {
			logger.Error(err, "html2pdf", "file", fn)
			errch <- err
			continue
		}
		files <- ArchFileItem{Filename: fn}
	}

	logger.Info("Save unused cids", "cids", len(cids))
	// Add unused
	for _, part := range cids {
		logger.Info("cid", "used", part.used)
		if !part.used {
			outch <- *part.MailPart
		}
	}
}

var htmlConverter Converter

func html2pdf(ctx context.Context, fn string, alter string, aConverter Converter) (string, error) {
	if fn == "" {
		logger.Info("empty filename!!!")
		return "", errors.New("empty filename")
	}
	logger := logger.WithName("html2pdf").WithValues("html", fn)
	dn, bn := filepath.Split(fn)
	destfn := filepath.Join(dn, bn+".pdf")
	fh, err := os.Open(fn)
	if err != nil {
		logger.Error(err, "open", "file", fn)
		err = fmt.Errorf("open html %s: %w", fn, err)
	} else {
		if htmlConverter == nil {
			htmlConverter = GetConverter(textHtml, nil)
		}
		err = htmlConverter(ctx, destfn, fh, textHtml)
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

type usedPart struct {
	*i18nmail.MailPart
	used bool
}

func embedCids(r io.Reader, cids map[string]*usedPart) (io.Reader, error) {
	mimes := make(map[string]string, len(cids))
	for k, v := range cids {
		if _, ok := mimes[k]; !ok {
			var a [1024]byte
			n, err := v.GetBody().ReadAt(a[:], 0)
			if n == 0 {
				if err != nil && !errors.Is(err, io.EOF) {
					return r, err
				}
				continue
			}
			ct, _ := MIMEMatch(a[:n])
			mimes[k] = ct
		}
	}
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		out := bufio.NewWriter(pw)
		defer out.Flush()
		var buf bytes.Buffer
		br := bufio.NewReaderSize(r, 16<<20)
		for {
			b, err := br.ReadSlice('>')

			buf.Reset()
			for {
				i := bytes.Index(b, []byte(`src="cid:`))
				if i < 0 {
					out.Write(buf.Bytes())
					out.Write(b)
					break
				}
				i += 5
				buf.Write(b[:i])
				b = b[i:]
				j := bytes.IndexByte(b, '"')
				if j < 0 {
					out.Write(buf.Bytes())
					out.Write(b)
					break
				}
				k := string(b[4:j])
				b = b[j+1:]
				r := cids[k]
				if r == nil {
					logger.Info("cid not found", "k", k)
					buf.WriteString("cid:" + k + `"`)
					continue
				}
				r.used = true
				out.Write(buf.Bytes())
				buf.Reset()
				out.WriteString(`data:`)
				out.WriteString(mimes[k])
				out.WriteString(";base64,")
				if _, err := io.Copy(base64.NewEncoder(base64.StdEncoding, out), r.GetBody()); err != nil {
					pw.CloseWithError(fmt.Errorf("read cid=%q: %w", k, err))
					return
				}
				out.WriteByte('"')
			}

			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				logger.Error(err, "readslice")
				pw.CloseWithError(err)
				return
			}
		}
	}()
	return pr, nil
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
				} else if part, err = part.WithBody(fh); err != nil {
					errch <- err
					continue
				}

			}
		}
		outch <- part
	}
}
