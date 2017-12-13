// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"context"

	"github.com/pkg/errors"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"

	"github.com/tgulacsi/go/i18nmail"
)

type cidGroup struct {
	htmlFn string
	cids   map[string]string
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
	Log := getLogger(ctx).Log
	defer func() {
		close(outch)
	}()
	ctx, wd := prepareContext(ctx, "")

	var (
		alter      = ""
		converter  = GetConverter("text/html", nil)
		aConverter Converter
		tbd        = make(map[string]struct{}, 4)
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
			Log("msg", "empty filename!!!")
			return "", errors.New("empty filename!")
		}
		tbd[fn] = struct{}{}
		destfn := filepath.Join(wd, filepath.Base(fn)+".pdf")
		fh, err := os.Open(fn)
		if err != nil {
			err = errors.Wrapf(err, "open html %s", fn)
		} else {
			if err = converter(ctx, destfn, fh, "text/html"); err != nil {
				err = errors.Wrapf(err, "converting %s to %s", fn, destfn)
			}
		}
		if err != nil {
			Log("msg", "html2pdf", "error", err)
			if alter != "" && aConverter != nil {
				Log("msg", "html2pdf using alternative content "+alter)
				if fh, err = os.Open(alter); err != nil {
					err = errors.Wrapf(err, "open txt %s", alter)
				} else {
					if err = aConverter(ctx, destfn, fh, "text/plain"); err != nil {
						err = errors.Wrapf(err, "converting %s to %s", alter, destfn)
					}
				}
				alter, aConverter = "", nil
			}
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
	last = -1
	for part := range inch {

		this = -1
		parent = part.Parent
		if parent == nil {
			grandpa = nil
		} else {
			grandpa = parent.Parent
		}
		if part.ContentType == "text/plain" || part.ContentType == "text/html" {
			//if part.Parent.ContentType != "multipart/alternative" || part.Parent.ContentType != "multipart/related" {
			if part.ContentType == "text/plain" && part.Parent.ContentType != "multipart/alternative" {
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
			_ = os.Mkdir(dn, 0755) //ignore errors
			if parent != nil {
				fn = fmt.Sprintf("%02d#%03d.index", parent.Level, parent.Seq)
			} else {
				fn = fmt.Sprintf("%02d#%03d.index", part.Level, part.Seq)
			}

			var cids map[string]string
			if part.ContentType == "text/plain" {
				fn = fn + ".txt"
			} else {
				part.Body = fixXMLCharset(ctx, fixXMLHeader(part.Body))
				cids = make(map[string]string, 4)
				part.Body = NewCidMapper(cids, "images", part.Body)
				fn = fmt.Sprintf("%s-%02d.html", fn, this)
			}
			fn = filepath.Join(dn, fn)
			if err = writeToFile(ctx, fn, part.Body, part.ContentType /*, mailHeader*/); err != nil {
				goto Error
			}
			tbd[fn] = struct{}{}
			if part.ContentType == "text/plain" {
				alter = fn
				aConverter = GetConverter("text/plain", part.MediaType)
			} else if part.ContentType == "text/html" {
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
				err = errors.New(fmt.Sprintf("WARN this=%d not in %v", part.Seq, groups))
				Log("msg", "SKIP not found", "error", err)
				goto Skip
			}
		}

		// K-MT9641 skip images which aren't in the .html
		for _, cg := range groups[this] {
			if fn, ok = cg.cids[cid]; !ok {
				goto Skip
			}
			fn = filepath.Join(filepath.Dir(cg.htmlFn), fn)

			_ = os.Mkdir(filepath.Dir(fn), 0755) // ignore error
			Log("save", fn, "cid", cid, "htmlFn", cg.htmlFn)
			if err = writeToFile(ctx, fn, part.Body, part.ContentType /*, mailHeader*/); err != nil {
				goto Error
			}
			tbd[filepath.Dir(fn)] = struct{}{}
		}

		continue
	Error:
		Log("error", err)
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
				getLogger(ctx).Log("msg", "get decoder for", "charset", cs, "error", err)
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
	Log := getLogger(ctx).Log
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
		if part.ContentType == "text/html" {
			fn := filepath.Join(wd, fmt.Sprintf("%02d#%03d.ori.html", part.Level, part.Seq))
			Log("msg", "saving original html "+fn)
			if orifh, e := os.Create(fn); e == nil {
				if _, e = io.Copy(orifh, part.Body); e != nil {
					Log("msg", "write ori to", "dest", orifh.Name(), "error", e)
				}
				orifh.Close()
				if part.Body, e = os.Open(orifh.Name()); e != nil {
					Log("msg", "reopen", "file", orifh.Name(), "error", e)
					errch <- e
					continue
				}
			}
		}
		outch <- part
	}
}
