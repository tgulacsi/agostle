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
	"testing"
	"time"
	"unicode/utf16"

	"context"

	"github.com/tgulacsi/go/text"
)

var accented = `
Árvíztűrő
 tükörfúrógép`

var testDir string

func TestMain(m *testing.M) {
	var err error
	testDir, err = os.MkdirTemp("", "agostle-test-")
	if err != nil {
		fmt.Println(err)
		os.Exit(13)
	}
	code := m.Run()
	_ = os.RemoveAll(testDir)
	os.Exit(code)
}

func tempFile(fn string) (*os.File, error) {
	fn = filepath.Base(fn)
	ext := filepath.Ext(fn)
	bn := fn[:len(fn)-len(ext)]

	return os.CreateTemp(testDir, bn+"-*"+ext)
}

func TestText(t *testing.T) {
	out, err := tempFile("a.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(out.Name())
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
	out, err := tempFile("b.html")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("out=%q", out.Name())
	defer os.Remove(out.Name())
	defer out.Close()

	_, _ = io.WriteString(out, `<!DOCTYPE html>
<html lang="hu" />
<head><meta charset="utf-8" /><title>proba</title></head>
<body>`)
	_, _ = io.WriteString(out, `<p>`+accented+`</p>`)
	_, _ = io.WriteString(out, `</body></html>`)
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = lofficeConvert(ctx, testDir, out.Name())
	cancel()
	if err != nil {
		t.Errorf("error converting with loffice: %s", err)
	}
}
