// Copyright 2013 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bufio"
	"bytes"
	"io"
	"strings"

	"golang.org/x/net/context"

	"github.com/tgulacsi/go/i18nmail"
)

// TextDecodeFilter writes Subject, From... headers at the beginning of the html/plain parts.
func TextDecodeFilter(ctx context.Context,
	inch <-chan i18nmail.MailPart, outch chan<- i18nmail.MailPart,
	files chan<- ArchFileItem, errch chan<- error,
) {
	//Log := getLogger(ctx).Log
	defer func() {
		close(outch)
	}()

	cet := "Content-Transfer-Encoding"
	ctx, _ = prepareContext(ctx, "")
	for part := range inch {
		// decode text/plain and text/html
		if part.ContentType == "text/plain" || part.ContentType == "text/html" {
			// QUOTED-PRINTABLE
			if part.Header.Get(cet) != "quoted-printable" &&
				strings.ToLower(part.Header.Get(cet)) == "quoted-printable" {
				part.Body = NewQuoPriDecoder(part.Body)
				part.Header.Del(cet)
			}

			br := bufio.NewReader(part.Body)
			var r = io.Reader(br)
			buf, _ := br.Peek(1024)
			i := bytes.Index(buf, []byte("+A"))
			if i >= 0 && bytes.IndexByte(buf[i+2:], '-') >= 0 {
				r = NewB64QuoPriDecoder(r)
			} else if bytes.Contains(buf, []byte("=0A=")) {
				r = NewQuoPriDecoder(r)
			}
			part.Body = r

			if part.ContentType == "text/plain" {
				part.Body = NewTextReader(ctx, part.Body, part.MediaType["charset"])
				part.MediaType["charset"] = "utf-8"
			}
		}

		outch <- part
	}

}
