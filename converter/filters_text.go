// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bufio"
	"bytes"
	"io"
	"strings"

	"context"

	"github.com/tgulacsi/go/i18nmail"
)

// TextDecodeFilter writes Subject, From... headers at the beginning of the html/plain parts.
func TextDecodeFilter(ctx context.Context,
	inch <-chan i18nmail.MailPart, outch chan<- i18nmail.MailPart,
	files chan<- ArchFileItem, errch chan<- error,
) {
	logger := getLogger(ctx).WithName("TextDecodeFilter")
	defer func() {
		close(outch)
	}()

	cet := "Content-Transfer-Encoding"
	ctx, _ = prepareContext(ctx, "")
	for part := range inch {
		// decode text/plain and text/html
		if part.ContentType == textPlain || part.ContentType == textHtml {
			// QUOTED-PRINTABLE
			if part.Header.Get(cet) != "quoted-printable" &&
				strings.ToLower(part.Header.Get(cet)) == "quoted-printable" {
				part.Header.Del(cet)
				part, _ = part.WithBody(NewQuoPriDecoder(part.GetBody()))
			}

			br := bufio.NewReader(part.GetBody())
			r := io.Reader(br)
			var changed bool
			buf, _ := br.Peek(1024)
			i := bytes.Index(buf, []byte("+A"))
			if i >= 0 && bytes.IndexByte(buf[i+2:], '-') >= 0 {
				r = NewB64QuoPriDecoder(r)
				changed = true
			} else if bytes.Contains(buf, []byte("=0A=")) {
				r = NewQuoPriDecoder(r)
				changed = true
			}
			if changed {
				part, _ = part.WithBody(r)
			}

			if part.ContentType == textPlain {
				part, _ = part.WithBody(NewTextReader(ctx, part.GetBody(), part.MediaType["charset"]))
				if part.MediaType == nil {
					part.MediaType = map[string]string{"charset": "utf-8"}
				} else {
					part.MediaType["charset"] = "utf-8"
				}
			}
		}

		select {
		case outch <- part:
		case <-ctx.Done():
			logger.Error(ctx.Err(), "TextDecodeFilter out")
			return
		}
	}

}
