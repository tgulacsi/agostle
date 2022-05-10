// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"io"

	"context"

	"github.com/tgulacsi/go/text"
)

var WriteTextAsPDF func(w io.Writer, r io.Reader) error

// NewTextReader wraps a reader with a proper charset converter
func NewTextReader(ctx context.Context, r io.Reader, charset string) io.Reader {
	if charset == "" || charset == "utf-8" {
		return text.NewReader(r, nil)
	}
	enc := text.GetEncoding(charset)
	if enc == nil {
		getLogger(ctx).Info("no decoder for", "charset", charset)
		return r
	}
	return text.NewReader(r, enc)
}

// NewTextConverter converts encoded text to pdf - by decoding it
func NewTextConverter(charset string) Converter {
	return func(ctx context.Context, destfn string, r io.Reader, contentType string) error {
		return TextToPdf(ctx, destfn, NewTextReader(ctx, r, charset), contentType)
	}
}
