// Copyright 2015 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/kylelemons/godebug/diff"
)

const pdfFormExample = "testdata/OoPdfFormExample.pdf"

func TestDumpFields(t *testing.T) {
	defer setTestLogger(t)()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	fields, err := PdfDumpFields(ctx, pdfFormExample)
	cancel()
	if err != nil {
		t.Fatalf("PdfDumpFields: %v", err)
	}
	t.Logf("fields=%q", fields)
}

func TestGetFdf(t *testing.T) {
	defer setTestLogger(t)()
	var err error
	if Workdir, err = os.MkdirTemp(testDir, "agostle-"); err != nil {
		t.Fatalf("tempdir for Workdir: %v", err)
	}
	defer os.RemoveAll(Workdir)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	s := time.Now()
	fp1, err := getFdf(ctx, pdfFormExample)
	cancel()
	t.Logf("PDF -> FDF vanilla route: %s (%d fields)", time.Since(s), len(fp1.Fields))
	if err != nil {
		t.Errorf("getFdf: %v", err)
	}
	if len(fp1.Fields) != 4 {
		t.Errorf("getFdf: got %d, awaited %d fields.", len(fp1.Fields), 4)
	}
	var buf1 bytes.Buffer
	if _, err = fp1.WriteTo(&buf1); err != nil {
		t.Errorf("WriteTo1: %v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	s = time.Now()
	fp2, err := getFdf(ctx, pdfFormExample)
	cancel()
	t.Logf("gob -> FDF route: %s", time.Since(s))
	if err != nil {
		t.Errorf("getFdf2: %v", err)
	}
	if !reflect.DeepEqual(fp1, fp2) {
		t.Errorf("getFdf2: got %#v, awaited %#v.", fp2, fp1)
	}
	var buf2 bytes.Buffer
	if _, err = fp2.WriteTo(&buf2); err != nil {
		t.Errorf("WroteTo2: %v", err)
	}
	if df := diff.Diff(buf1.String(), buf2.String()); df != "" {
		t.Errorf("DIFF: %s", df)
	}

	out, err := Exec.CommandContext(context.Background(), "ls", "-l", Workdir).Output()
	if err == nil {
		t.Logf("ls -l %s:\n%s", Workdir, out)
	}
}

func TestSplitFdf(t *testing.T) {
	fdf := []byte(`%FDF-1.2
%âăĎÓ
1 0 obj
<<
/FDF
<<
/Fields [
<<
/V ()
/T (Forgalmi rendszámRow4)
>>
<<
/V ()
/T (Forgalmi rendszámRow3)
>>
<<
/V ()
/T (Forgalmi rendszámRow2)
>>
<<
/V ()
/T (Az Ön gépjárművének forgalmi rendszáma)
>>
<<
/V ()
/T (Forgalmi rendszámRow1)
>>
<<
/V ()
/T (neve_2)
>>
<<
/V ()
/T (neve_3)
>>
<<
/V ()
/T (neve)
>>
<<
/V ()
/T (neve_4)
>>
<<
/V ()
/T (házszám)
>>
<<
/V ()
/T (SérülésRow1)
>>
<<
/V ()
/T (SérülésRow2)
>>
<<
/V ()
/T (SérülésRow3)
>>
<<
/V ()
/T (címe_2)
>>
<<
/V ()
/T (címe_3)
>>
<<
/V ()
/T (címe_4)
>>
<<
/V ()
/T (Cím telefonszámRow3)
>>
<<
/V ()
/T (Cím telefonszámRow2)
>>
<<
/V ()
/T (Cím telefonszámRow1)
>>
<<
/V ()
/T (Email)
>>
<<
/V ()
/T (NévRow1)
>>
<<
/V ()
/T (NévRow2)
>>
<<
/V ()
/T (NévRow3)
>>
<<
/V ()
/T (ututca)
>>
<<
/V ()
/T (Text21)
>>
<<
/V ()
/T (címe)
>>
<<
/V ()
/T (Text22)
>>
<<
/V ()
/T (Text23)
>>
<<
/V ()
/T (Text24)
>>
<<
/V ()
/T (Text25)
>>
<<
/V ()
/T (Text26)
>>
<<
/V ()
/T (Text27)
>>
<<
/V ()
/T (Tett intézkedés kérjük csatolja az igazoló dokumentumot)
>>
<<
/V ()
/T (Lakott területen kívül)
>>
<<
/V ()
/T (biztosítónál)
>>
<<
/V ()
/T (évhónap)
>>
<<
/V ()
/T (Sérülések leírásaRow6)
>>
<<
/V ()
/T (Sérülések leírásaRow5)
>>
<<
/V ()
/T (Sérülések leírásaRow4)
>>
<<
/V ()
/T (Sérülések leírásaRow3)
>>
<<
/V ()
/T (Sérülések leírásaRow2)
>>
<<
/V ()
/T (Sérülések leírásaRow1)
>>
<<
/V ()
/T (Milyen minôségben vezette a gépjárművet kérjük szíveskedjen az igazoló dokumentumot is csatolni pl kölcsönadási szerzôdés stb)
>>
<<
/V ()
/T (kerület)
>>
<<
/V ()
/T (város)
>>
<<
/V ()
/T (TípusRow1)
>>
<<
/V ()
/T (TípusRow2)
>>
<<
/V ()
/T (út)
>>
<<
/V ()
/T (TípusRow3)
>>
<<
/V ()
/T (TípusRow4)
>>
<<
/V ()
/T (TípusRow5)
>>
<<
/V ()
/T (undefined_2)
>>
<<
/V ()
/T (TípusRow6)
>>
<<
/V ()
/T (undefined_3)
>>
<<
/V ()
/T (undefined_4)
>>
<<
/V /
/T (Check Box20)
>>
<<
/V ()
/T (Kelt)
>>
<<
/V ()
/T (Cím)
>>
<<
/V ()
/T (Járműszerelvény esetén a pótkocsi forgalmi rendszáma)
>>
<<
/V ()
/T (óraperc)
>>
<<
/V ()
/T (Telefon)
>>
<<
/V /
/T (Check Box19)
>>
<<
/V /
/T (Check Box18)
>>
<<
/V /
/T (Check Box17)
>>
<<
/V ()
/T (A baleset helye)
>>
<<
/V /
/T (Check Box16)
>>
<<
/V ()
/T (Forgalmi rendszámRow6)
>>
<<
/V /
/T (Check Box15)
>>
<<
/V ()
/T (Forgalmi rendszámRow5)
>>]
>>
>>
endobj
trailer

<<
/Root 1 0 R
>>
%%EOF
`)

	fp := splitFdf(fdf)
	if len(fp.Parts) != 64 {
		t.Errorf("wanted 64 parts, got %d", len(fp.Parts))
	}
	if len(fp.Fields) != 63 {
		t.Errorf("wanted 63 fields, got %d", len(fp.Fields))
	}
	t.Logf("splitted=%q (%d)", fp, len(fp.Parts))

	if err := fp.Set("A baleset helye", "Kiskunbürgözd"); err != nil {
		t.Errorf("Set: %v", err)
	}

	var buf bytes.Buffer
	if _, err := fp.WriteTo(&buf); err != nil {
		t.Errorf("WriteTo: %v", err)
	}

	if df := diff.Diff(`%FDF-1.2
%âăĎÓ
1 0 obj
<<
/FDF
<<
/Fields [
<<
/V ()
/T (Forgalmi rendszámRow4)
>>
<<
/V ()
/T (Forgalmi rendszámRow3)
>>
<<
/V ()
/T (Forgalmi rendszámRow2)
>>
<<
/V ()
/T (Az Ön gépjárművének forgalmi rendszáma)
>>
<<
/V ()
/T (Forgalmi rendszámRow1)
>>
<<
/V ()
/T (neve_2)
>>
<<
/V ()
/T (neve_3)
>>
<<
/V ()
/T (neve)
>>
<<
/V ()
/T (neve_4)
>>
<<
/V ()
/T (házszám)
>>
<<
/V ()
/T (SérülésRow1)
>>
<<
/V ()
/T (SérülésRow2)
>>
<<
/V ()
/T (SérülésRow3)
>>
<<
/V ()
/T (címe_2)
>>
<<
/V ()
/T (címe_3)
>>
<<
/V ()
/T (címe_4)
>>
<<
/V ()
/T (Cím telefonszámRow3)
>>
<<
/V ()
/T (Cím telefonszámRow2)
>>
<<
/V ()
/T (Cím telefonszámRow1)
>>
<<
/V ()
/T (Email)
>>
<<
/V ()
/T (NévRow1)
>>
<<
/V ()
/T (NévRow2)
>>
<<
/V ()
/T (NévRow3)
>>
<<
/V ()
/T (ututca)
>>
<<
/V ()
/T (Text21)
>>
<<
/V ()
/T (címe)
>>
<<
/V ()
/T (Text22)
>>
<<
/V ()
/T (Text23)
>>
<<
/V ()
/T (Text24)
>>
<<
/V ()
/T (Text25)
>>
<<
/V ()
/T (Text26)
>>
<<
/V ()
/T (Text27)
>>
<<
/V ()
/T (Tett intézkedés kérjük csatolja az igazoló dokumentumot)
>>
<<
/V ()
/T (Lakott területen kívül)
>>
<<
/V ()
/T (biztosítónál)
>>
<<
/V ()
/T (évhónap)
>>
<<
/V ()
/T (Sérülések leírásaRow6)
>>
<<
/V ()
/T (Sérülések leírásaRow5)
>>
<<
/V ()
/T (Sérülések leírásaRow4)
>>
<<
/V ()
/T (Sérülések leírásaRow3)
>>
<<
/V ()
/T (Sérülések leírásaRow2)
>>
<<
/V ()
/T (Sérülések leírásaRow1)
>>
<<
/V ()
/T (Milyen minôségben vezette a gépjárművet kérjük szíveskedjen az igazoló dokumentumot is csatolni pl kölcsönadási szerzôdés stb)
>>
<<
/V ()
/T (kerület)
>>
<<
/V ()
/T (város)
>>
<<
/V ()
/T (TípusRow1)
>>
<<
/V ()
/T (TípusRow2)
>>
<<
/V ()
/T (út)
>>
<<
/V ()
/T (TípusRow3)
>>
<<
/V ()
/T (TípusRow4)
>>
<<
/V ()
/T (TípusRow5)
>>
<<
/V ()
/T (undefined_2)
>>
<<
/V ()
/T (TípusRow6)
>>
<<
/V ()
/T (undefined_3)
>>
<<
/V ()
/T (undefined_4)
>>
<<
/V /
/T (Check Box20)
>>
<<
/V ()
/T (Kelt)
>>
<<
/V ()
/T (Cím)
>>
<<
/V ()
/T (Járműszerelvény esetén a pótkocsi forgalmi rendszáma)
>>
<<
/V ()
/T (óraperc)
>>
<<
/V ()
/T (Telefon)
>>
<<
/V /
/T (Check Box19)
>>
<<
/V /
/T (Check Box18)
>>
<<
/V /
/T (Check Box17)
>>
<<
/V (`+"\xfe\xff\x00K\x00i\x00s\x00k\x00u\x00n\x00b\x00\xfc\x00r\x00g\x00\xf6\x00z\x00d"+`)
/T (A baleset helye)
>>
<<
/V /
/T (Check Box16)
>>
<<
/V ()
/T (Forgalmi rendszámRow6)
>>
<<
/V /
/T (Check Box15)
>>
<<
/V ()
/T (Forgalmi rendszámRow5)
>>]
>>
>>
endobj
trailer

<<
/Root 1 0 R
>>
%%EOF
`,
		buf.String(),
	); df != "" {
		t.Errorf("mismatch: %s", df)
	}
}
