// Copyright 2017, 2023 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"net/mail"
	"net/textproto"
	"os"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"

	"github.com/tgulacsi/go/byteutil"
	"github.com/tgulacsi/go/i18nmail"
)

// PrependHeaderFilter writes Subject, From... headers at the beginning of the html/plain parts.
func PrependHeaderFilter(ctx context.Context,
	inch <-chan i18nmail.MailPart, outch chan<- i18nmail.MailPart,
	files chan<- ArchFileItem, errch chan<- error,
) {
	logger := getLogger(ctx)
	defer func() {
		close(outch)
	}()
	ctx, _ = PrepareContext(ctx, "")

	mailHeader := mail.Header(make(map[string][]string, len(PrependHeaders)))
	headersBuf := bytes.NewBuffer(make([]byte, 0, 128))

	for part := range inch {
		logger.Debug("PrependHeaderFilter receives", "seq", part.Seq, "ct", part.ContentType, "header", part.Header, "inch", inch)
		if len(mailHeader["From"]) == 0 || mailHeader.Get("Subject") == "" {
			hdrs := make([]textproto.MIMEHeader, 0, 4)
			{
				parent := &part
				if len(parent.Header) > 0 {
					hdrs = append(hdrs, parent.Header)
				}
				for i := 0; parent.Parent != nil && parent.Parent != parent && i < 10; i++ {
					parent = parent.Parent
					if len(parent.Header) > 0 {
						logger.Debug("parent", "header", parent.Header)
						hdrs = append(hdrs, parent.Header)
					}
				}
			}
			if len(hdrs) > 0 {
				hdr := hdrs[len(hdrs)-1]
				logger.Info("filling mailHeader", "header", hdr)
				for _, k := range PrependHeaders {
					if v, ok := hdr[k]; ok {
						mailHeader[k] = v
					}
				}
			}
		}

		if !(part.ContentType == textPlain || part.ContentType == textHtml) {
			goto Skip
		}
		headersBuf.Reset()
		if err := writeHeaders(ctx, headersBuf, mailHeader, part.ContentType); err != nil {
			logger.Info("error writing headers", "error", err)
			goto Skip
		}
		//logger.Info("headers written", "buf", headersBuf.String())

		if part.ContentType != textHtml {
			part.Body, _ = i18nmail.MakeSectionReader(
				io.MultiReader(bytes.NewReader(headersBuf.Bytes()), part.Body),
				bodyThreshold,
			)
			goto Skip
		}

		part.Body, _ = i18nmail.MakeSectionReader(
			decodeHTML(ctx, part.Body, *ConfWkhtmltopdf == ""), bodyThreshold)
		logger.Debug("PrependHeaderFilter", "wkhtmltopdf", *ConfWkhtmltopdf, "body", part.Body, "threshold", bodyThreshold)
		if *ConfWkhtmltopdf == "" {
			b, err := io.ReadAll(part.Body)
			if err != nil {
				logger.Info("cannot read", "body", part.Body, "error", err)
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
			if bodyPos, _ := tagIndex(b, "body"); bodyPos >= 0 {
				// even more garbage
				b = append(append(b[:bodyPos], []byte("<!-- ------ -->")...),
					b[bodyPos:]...)
			}

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
				part.Body, _ = i18nmail.MakeSectionReader(io.MultiReader(
					bytes.NewReader(b[:j]),
					bytes.NewReader(headersBuf.Bytes()),
					bytes.NewReader(b[j:]),
				), bodyThreshold)
			} else {
				part.Body, _ = i18nmail.MakeSectionReader(io.MultiReader(
					bytes.NewReader(headersBuf.Bytes()),
					bytes.NewReader(b),
				), bodyThreshold)
			}
		} else {
			b := make([]byte, 1<<20)
			n, _ := io.ReadAtLeast(part.Body, b, len(b)/2)
			b = b[:n]
			if _, j := tagIndex(b, "body"); j >= 0 {
				part.Body, _ = i18nmail.MakeSectionReader(io.MultiReader(
					bytes.NewReader(b[:j]),
					bytes.NewReader(headersBuf.Bytes()),
					bytes.NewReader(b[j:]),
					part.Body,
				), bodyThreshold)
			} else {
				c := b
				if len(c) > 4096 {
					c = c[len(c)-4096:]
				}
				logger.Info("no body in", "b", string(c))
				part.Body, _ = i18nmail.MakeSectionReader(io.MultiReader(
					bytes.NewReader(headersBuf.Bytes()),
					bytes.NewReader(b),
					part.Body,
				), bodyThreshold)
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

	logger := getLogger(ctx)
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
		cs := bytes.TrimSpace(c[:k])
		if len(cs) == 0 {
			continue
		}
		charset := string(cs)
		if deleteMETA {
			// delete the whole <meta .../> part
			copy(b[i:j], bytes.Repeat([]byte{' '}, j-i))
		}
		var err error
		if enc, err = htmlindex.Get(charset); err != nil {
			logger.Info("cannot find encoding", "charset", charset)
			continue
		}
		if !deleteMETA && len(charset) >= 5 {
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
		return fmt.Errorf("create file %s: %w", fn, err)
	}
	br := bufio.NewReader(r)

	logger := getLogger(ctx)
	logger.Info("writeToPdfFile", "file", fn, "ct", contentType)
	if _, err = io.Copy(fh, br); err != nil {
		_ = fh.Close()
		return fmt.Errorf("save to %s: %w", fn, err)
	}
	return fh.Close()
}

func writeHeaders(ctx context.Context, w io.Writer, mailHeader mail.Header, contentType string) error {
	logger := getLogger(ctx)
	if mailHeader == nil || !(contentType == textPlain || contentType == textHtml) {
		return nil
	}
	var buf bytes.Buffer
	ew := &errWriter{Writer: &buf}
	var preList, postList, preKey string
	postKey, preAddr, postAddr := ": ", "<", ">"
	bol, eol := "  ", "\r\n"
	escape := func(s string) string { return s }

	if contentType == textHtml {
		preList, postList = "<div><ul>\n", "</ul></div>\n"
		preKey, postKey = "<em>", ":</em> "
		bol, eol = "<li>", "</li>\n"
		preAddr, postAddr = "&lt;", "&gt;"
		escape = template.HTMLEscapeString
	}

	_, _ = io.WriteString(ew, preList)
	mh := i18nmail.Header(mailHeader)
	for _, k := range PrependHeaders {
		if k == "Subject" || k == "Date" {
			continue
		}
		if mh.Get(k) == "" {
			continue
		}
		_, _ = io.WriteString(ew, bol+preKey+k+postKey)
		addr, err := mh.AddressList(k)
		if err != nil {
			if a, err := i18nmail.ParseAddress(mh.Get(k)); err != nil {
				logger.Info("parsing address", "of", k, "value", mh.Get(k), "error", err)
				_, _ = io.WriteString(ew, escape(mh.Get(k)))
			} else {
				addr = append(addr, a)
			}
		}
		s := ""
		for i, a := range addr {
			_, _ = io.WriteString(ew, s+escape(a.Name)+" "+preAddr+escape(a.Address)+postAddr)
			if i == 0 {
				s = ", "
			}
		}
		_, _ = io.WriteString(ew, eol)
	}
	_, _ = io.WriteString(ew, bol+preKey+"Subject"+postKey+escape(i18nmail.HeadDecode(mh.Get("Subject")))+eol)
	_, _ = io.WriteString(ew, bol+preKey+"Date"+postKey+escape(mh.Get("Date"))+eol)
	_, _ = io.WriteString(ew, postList)

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
