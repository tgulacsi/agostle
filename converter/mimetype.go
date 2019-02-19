// Copyright 2019 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"github.com/h2non/filetype"
	filetypes "github.com/h2non/filetype/types"
)

type h2nonMIMEDetector struct{}

type MIMEDetector interface {
	Match([]byte) (string, error)
}

func NewMIMEDetector() (MIMEDetector, error) {
	return h2nonMIMEDetector{}, nil
}

var DefaultMIMEDetector = MIMEDetector(h2nonMIMEDetector{})

func MIMEMatch(b []byte) (string, error) { return DefaultMIMEDetector.Match(b) }

func (d h2nonMIMEDetector) Close() error { return nil }
func (d h2nonMIMEDetector) Match(b []byte) (string, error) {
	typ, err := filetype.Match(b)
	if typ == filetypes.Unknown {
		return "", err
	}
	return typ.MIME.Type + "/" + typ.MIME.Subtype, err
}
