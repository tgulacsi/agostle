// Copyright 2023 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"io"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// ImageToPdfPdfCPU converts image to PDF using pdfcpu
func ImageToPdfPdfCPU(w io.Writer, r io.Reader) error {
	return api.ImportImages(nil, w, []io.Reader{r}, nil, nil)
}
