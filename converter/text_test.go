// Copyright 2013 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"
	"unicode/utf16"

	"context"

	//"bitbucket.org/zombiezen/gopdf/pdf"
	//"github.com/mawicks/PDFiG/pdf"
	//"github.com/signintech/gopdf/src/gopdf"
	"github.com/tgulacsi/go/text"
)

var accented = `
Árvíztűrő
 tükörfúrógép`

func TestText(t *testing.T) {
	out, err := os.Create("/tmp/a.pdf")
	if err != nil {
		t.Errorf("error creating out file /tmp/a.pdf: %s", err)
		return
	}
	defer out.Close()
	in := bytes.NewBuffer(nil)
	in.WriteString("UTF-8: ")
	in.WriteString(accented)
	sep := []byte("\n\n---\n\n")

	in.Write(sep)
	in.WriteString("UTF-16: ")
	u := utf16.Encode([]rune(accented))
	for i := 0; i < len(u); i++ {
		in.Write([]byte{byte(u[i] >> 8), byte(u[i] & 0xff)})
	}

	in.Write(sep)
	in.WriteString("ISO-8859-15: ")
	i15, _ := text.Encode(accented, text.GetEncoding("ISO-8859-15"))
	t.Logf("i15: %q", i15)
	in.Write(i15)

	in.Write(sep)
	in.WriteString("windows-1252: ")
	w1252, _ := text.Encode(accented, text.GetEncoding("windows-1252"))
	t.Logf("w1252: %q", w1252)
	in.Write(w1252)

	in.Write(sep)
	in.WriteString("macroman: ")
	mr, _ := text.Encode(accented, text.GetEncoding("macroman"))
	t.Logf("mr: %q", mr)
	in.Write(mr)

	in.Write(sep)
	if WriteTextAsPDF != nil {
		err = WriteTextAsPDF(out, bytes.NewReader(in.Bytes()))
		if err != nil {
			t.Errorf("error writing %q: %s", accented, err)
		}
	}
	t.Logf("out: %v", out)
}

func TestLoHtmlPdf(t *testing.T) {
	out, err := os.Create("/tmp/b.html")
	if err != nil {
		t.Errorf("error creating out file /tmp/b.html: %s", err)
		return
	}
	io.WriteString(out, `<!DOCTYPE html>
<html lang="hu" />
<head><meta charset="utf-8" /><title>proba</title></head>
<body>`)
	io.WriteString(out, `<p>`+accented+`</p>`)
	io.WriteString(out, `</body></html>`)
	out.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = lofficeConvert(ctx, "/tmp", "/tmp/b.html")
	cancel()
	if err != nil {
		t.Errorf("error converting with loffice: %s", err)
	}
	//os.Remove("/tmp/b.html")
}
