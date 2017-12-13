// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bufio"
	"bytes"
	"html/template"
	"io"
	"io/ioutil"
	"net/mail"
	"net/textproto"
	"os"

	"context"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"

	"github.com/pkg/errors"
	"github.com/tgulacsi/go/byteutil"
	"github.com/tgulacsi/go/i18nmail"
)

// PrependHeaderFilter writes Subject, From... headers at the beginning of the html/plain parts.
func PrependHeaderFilter(ctx context.Context,
	inch <-chan i18nmail.MailPart, outch chan<- i18nmail.MailPart,
	files chan<- ArchFileItem, errch chan<- error,
) {
	Log := getLogger(ctx).Log
	defer func() {
		close(outch)
	}()
	ctx, _ = prepareContext(ctx, "")

	mailHeader := mail.Header(make(map[string][]string, len(PrependHeaders)))
	headersBuf := bytes.NewBuffer(make([]byte, 0, 128))

	for part := range inch {
		//Log("msg", "PrependHeaderFilter receives", "seq", part.Seq, "ct", part.ContentType, "header", part.Header, "inch", inch)
		if len(mailHeader["From"]) == 0 || mailHeader["Subject"][0] == "" {
			hdrs := make([]textproto.MIMEHeader, 0, 4)
			parent := &part
			if len(parent.Header) > 0 {
				hdrs = append(hdrs, parent.Header)
			}
			for parent.Parent != nil {
				parent = parent.Parent
				if len(parent.Header) > 0 {
					hdrs = append(hdrs, parent.Header)
				}
			}
			if len(hdrs) > 0 {
				hdr := hdrs[len(hdrs)-1]
				Log("msg", "filling mailHeader", "header", hdr)
				for _, k := range PrependHeaders {
					if v, ok := hdr[k]; ok {
						mailHeader[k] = v
					}
				}
			}
		}

		if !(part.ContentType == "text/plain" || part.ContentType == "text/html") {
			goto Skip
		}
		headersBuf.Reset()
		if err := writeHeaders(ctx, headersBuf, mailHeader, part.ContentType); err != nil {
			Log("msg", "error writing headers", "error", err)
			goto Skip
		}
		//Log("msg", "headers written", "buf", headersBuf.String())

		if part.ContentType != "text/html" {
			part.Body = io.MultiReader(bytes.NewReader(headersBuf.Bytes()), part.Body)
			var buf bytes.Buffer
			io.Copy(&buf, part.Body)
			part.Body = bytes.NewReader(buf.Bytes())
			//Log("msg", buf.String())
			goto Skip
		}

		part.Body = decodeHTML(ctx, part.Body, *ConfWkhtmltopdf == "")
		if *ConfWkhtmltopdf == "" {
			b, err := ioutil.ReadAll(part.Body)
			if err != nil {
				Log("msg", "cannot read", "body", part.Body, "error", err)
			}
			// add some garbage to each line ending!
			b = bytes.TrimSpace(
				bytes.Replace(
					bytes.Replace(b, []byte("\r\n"), []byte{'\n'}, -1),
					[]byte{'\n'}, []byte("<!-- -->\n"), -1,
				))
			// change to HTML5
			i, _ := tagIndex(b, "head")
			if i < 0 {
				i, _ = tagIndex(b, "body")
			}
			// Insert DOCTYPE before <head
			if i >= 0 {
				const htmlPrefix = "<!DOCTYPE html>\n<html>\n"
				if i >= len(htmlPrefix) {
					b = b[i-len(htmlPrefix):]
					copy(b, []byte(htmlPrefix))
				} else {
					b = append([]byte(htmlPrefix), b[i:]...)
				}
			}
			bodyPos, _ := tagIndex(b, "body")
			// even more garbage
			b = append(append(b[:bodyPos], []byte("<!-- ------ -->")...),
				b[bodyPos:]...)

			if i := bytes.LastIndex(b, []byte("</body>")); i >= 0 {
				b = append(b[:i+7], []byte("</html>")...)
			} else if bytes.HasSuffix(b, []byte("</html")) {
				b = append(b, '>')
			} else if bytes.HasSuffix(b, []byte("</htm")) {
				b = append(b, 'l', '>')
			} else if bytes.HasSuffix(b, []byte("</ht")) {
				b = append(b, 'm', 'l', '>')
			}
			if _, j := tagIndex(b, "body"); j >= 0 {
				part.Body = io.MultiReader(
					bytes.NewReader(b[:j]),
					bytes.NewReader(headersBuf.Bytes()),
					bytes.NewReader(b[j:]),
				)
			} else {
				part.Body = io.MultiReader(
					bytes.NewReader(headersBuf.Bytes()),
					bytes.NewReader(b),
				)
			}
		} else {
			b := make([]byte, 4096)
			n, _ := io.ReadAtLeast(part.Body, b, len(b)/2)
			b = b[:n]
			if _, j := tagIndex(b, "body"); j >= 0 {
				part.Body = io.MultiReader(
					bytes.NewReader(b[:j]),
					bytes.NewReader(headersBuf.Bytes()),
					bytes.NewReader(b[j:]),
					part.Body,
				)
			} else {
				Log("msg", "no body in", "b", b)
				part.Body = io.MultiReader(
					bytes.NewReader(headersBuf.Bytes()),
					bytes.NewReader(b),
					part.Body,
				)
			}
		}

	Skip:
		outch <- part
	}
}

func tagIndex(b []byte, tag string) (int, int) {
	i := byteutil.ByteIndexFold(b, []byte("<"+tag))
	if i < 0 {
		return -1, -1
	}
	k := i + 1 + len(tag)
	j := bytes.IndexByte(b[k:], '>')
	if j < 0 {
		return -1, -1
	}
	j += k + 1
	return i, j
}

// decodeHTML decodes the HTML's encoding.
func decodeHTML(ctx context.Context, r io.Reader, deleteMETA bool) io.Reader {
	b := make([]byte, 4096)
	n, _ := io.ReadAtLeast(r, b, len(b)/2)
	b = b[:n]
	r = io.MultiReader(bytes.NewReader(b), r)

	Log := getLogger(ctx).Log
	var enc encoding.Encoding
	p := 0
	for {
		i, j := tagIndex(b[p:], "meta")
		if i < 0 {
			break
		}
		i, j = i+p, j+p
		p += j + 1

		c := b[i:j]
		k := byteutil.ByteIndexFold(c, []byte("charset="))
		if k < 0 {
			continue
		}
		c = c[k+8:]
		if len(c) > 0 && c[0] == '"' { // maybe HTML5 <meta charset="..."/>
			c = c[1:]
		}
		if k = bytes.IndexByte(c, '"'); k < 0 {
			continue
		}
		cs := c[:k]
		if len(cs) == 0 {
			continue
		}
		if deleteMETA {
			// delete the whole <meta .../> part
			copy(b[i:j], bytes.Repeat([]byte{' '}, j-i))
		}
		var err error
		if enc, err = htmlindex.Get(string(cs)); err != nil {
			Log("msg", "cannot find encoding for "+string(cs))
			continue
		}
		if !deleteMETA && len(cs) >= 5 {
			copy(c[:5], []byte("utf-8"))
			if len(cs) > 5 {
				copy(c[5:k], bytes.Repeat([]byte{' '}, k-5))
			}
		}
		break
	}
	if enc != nil {
		return transform.NewReader(r, enc.NewDecoder())
	}
	return r
}

// PrependHeaders are the headers which should be prepended to the printed mail
var PrependHeaders = []string{"From", "To", "Cc", "Subject", "Date"}

func writeToFile(ctx context.Context, fn string, r io.Reader, contentType string /*, mailHeader mail.Header*/) error {
	fh, err := os.Create(fn)
	if err != nil {
		return errors.Wrapf(err, "create file %s", fn)
	}
	br := bufio.NewReader(r)

	Log := getLogger(ctx).Log
	Log("msg", "writeToPdfFile", "file", fn, "ct", contentType)
	if _, err = io.Copy(fh, br); err != nil {
		_ = fh.Close()
		return errors.Wrapf(err, "save to %s", fn)
	}
	return fh.Close()
}

func writeHeaders(ctx context.Context, w io.Writer, mailHeader mail.Header, contentType string) error {
	Log := getLogger(ctx).Log
	if mailHeader == nil || !(contentType == "text/plain" || contentType == "text/html") {
		return nil
	}
	var buf bytes.Buffer
	ew := &errWriter{Writer: &buf}
	var preList, postList, preKey string
	postKey, preAddr, postAddr := ": ", "<", ">"
	bol, eol := "  ", "\r\n"
	escape := func(s string) string { return s }

	if contentType == "text/html" {
		preList, postList = "<div><ul>\n", "</ul></div>\n"
		preKey, postKey = "<em>", ":</em> "
		bol, eol = "<li>", "</li>\n"
		preAddr, postAddr = "&lt;", "&gt;"
		escape = template.HTMLEscapeString
	}

	io.WriteString(ew, preList)
	mh := i18nmail.Header(mailHeader)
	for _, k := range PrependHeaders {
		if k == "Subject" || k == "Date" {
			continue
		}
		if mh.Get(k) == "" {
			continue
		}
		io.WriteString(ew, bol+preKey+k+postKey)
		addr, err := mh.AddressList(k)
		if err != nil {
			if a, err := i18nmail.ParseAddress(mh.Get(k)); err != nil {
				Log("msg", "parsing address", "of", k, "value", mh.Get(k), "error", err)
				io.WriteString(ew, escape(mh.Get(k)))
			} else {
				addr = append(addr, a)
			}
		}
		s := ""
		for i, a := range addr {
			io.WriteString(ew, s+escape(a.Name)+" "+preAddr+escape(a.Address)+postAddr)
			if i == 0 {
				s = ", "
			}
		}
		io.WriteString(ew, eol)
	}
	io.WriteString(ew, bol+preKey+"Subject"+postKey+escape(i18nmail.HeadDecode(mh.Get("Subject")))+eol)
	io.WriteString(ew, bol+preKey+"Date"+postKey+escape(mh.Get("Date"))+eol)
	io.WriteString(ew, postList)

	if ew.Err != nil {
		return ew.Err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

type errWriter struct {
	io.Writer
	Err error
}

func (ew *errWriter) Write(p []byte) (int, error) {
	if ew.Err != nil {
		return 0, ew.Err
	}
	var n int
	n, ew.Err = ew.Writer.Write(p)
	return n, ew.Err
}
