// +build pdfdirect

// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"image"
	"io"

	_ "image/gif"  // to be able to open GIF files
	_ "image/jpeg" // to be able to open JPEG files
	_ "image/png"  // to be able to open PNG files

	_ "golang.org/x/image/bmp"  // to be able to open BMP files
	_ "golang.org/x/image/tiff" // to be able to open TIFF files

	"bitbucket.org/zombiezen/gopdf/pdf"
)

// ImageToPdfDirect converts image to PDF using gpdf
func ImageToPdfDirect(w io.Writer, r io.Reader) error {
	img, _, err := image.Decode(r)
	if err != nil {
		return err
	}

	ib := img.Bounds().Canon().Size()
	doc := pdf.New()
	canvas := doc.NewPage(pdf.A4Width, pdf.A4Height)
	canvas.Translate(pdf.Cm, canvas.CropBox().Max.Y-pdf.Cm)
	// TODO: A4 Portrait ratio vs. ib ratio
	ir := float32(ib.X) / float32(ib.Y)
	bb := canvas.CropBox()
	cbw, cbh := int(bb.Max.X-bb.Min.X), int(bb.Max.Y-bb.Min.Y)
	if ir > float32(cbw/cbh) {
		bb.Max.Y = bb.Min.Y + pdf.Unit(1.0/ir*float32(cbw))
	} else {
		bb.Max.X = bb.Min.X + pdf.Unit(ir*float32(cbh))
	}
	canvas.DrawImage(img, bb)
	if err = canvas.Close(); err != nil {
		return err
	}

	return doc.Encode(w)
}
