//go:build pdfdirect
// +build pdfdirect

// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bufio"
	"io"

	"bitbucket.org/zombiezen/gopdf/pdf"
)

func newPage(doc *pdf.Document) (canvas *pdf.Canvas, text *pdf.Text) {
	canvas = doc.NewPage(pdf.A4Width, pdf.A4Height)
	//canvas.SetCropBox(pdf.Rectangle{pdf.Point{pdf.Cm, pdf.Cm},
	//	pdf.Point{cb.Max.X - pdf.Cm, cb.Max.Y - pdf.Cm}})
	//cb = canvas.CropBox()
	//canvas.Translate(cb.Max.X, cb.Max.Y)
	text = new(pdf.Text)
	text.SetFont(pdf.Times, 14)
	canvas.Translate(pdf.Cm, canvas.CropBox().Max.Y-pdf.Cm)
	return
}

func init() {
	WriteTextAsPDF = writeTextAsPdf
}

// WriteTextAsPdf writes text to Pdf
func writeTextAsPdf(w io.Writer, r io.Reader) error {
	doc := pdf.New()

	canvas, text := newPage(doc)

	br := bufio.NewReader(r)
	m := canvas.CropBox().Max
	maxrowchars := int((m.X - 3*pdf.Cm) / 6)
	for line, err := br.ReadString('\n'); err == nil; {
		//fmt.Fprintln(os.Stderr, text.Y()+m.Y)
		//line = line + line
		if text.Y()+m.Y < 3*pdf.Cm {
			canvas.DrawText(text)
			if err = canvas.Close(); err != nil {
				return err
			}
			canvas, text = newPage(doc)
		}
		//for i := 0; i < 3; i++ {
		for j, k, n := 0, maxrowchars, len(line)-1; j < n; k += maxrowchars {
			if k > n {
				k = n
			}
			//fmt.Fprintf(os.Stderr, "n=%d j=%d k=%d\n", n, j, k)
			text.Text(line[j:k])
			text.NextLine()
			j = k
		}
		//}
		line, err = br.ReadString('\n')
	}
	canvas.DrawText(text)
	//fmt.Fprintf(os.Stderr, "maxlen=%d = maxwidth=%f\n", maxlen, maxwidth)

	if err := canvas.Close(); err != nil {
		return err
	}

	return doc.Encode(w)
}
