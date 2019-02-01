// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tgulacsi/go/iohlp"
)

func TestFixXMLHeader(t *testing.T) {
	want := `<?xml version="1.0" charset="utf-8"?><!DOCTYPE html></html>`
	for i, elt := range []string{
		want,
		strings.Replace(want, "charset", "encoding", 1),
	} {
		b, err := iohlp.ReadAll(fixXMLHeader(strings.NewReader(elt)), 1<<20)
		if err != nil {
			t.Errorf("%d. read: %v", i, err)
			continue
		}
		got := string(b)
		if got != want {
			t.Errorf("%d. got %q, want %q.", i, got, want)
		}
	}
}

func TestFixXMLCharset(t *testing.T) {
	want := `<?xml version="1.0" ?><!DOCTYPE html><p>á</p></html>`
	ctx := context.Background()
	for i, elt := range []string{
		want,
		`<?xml version="1.0" charset="utf-8" ?>` + want[22:],
		`<?xml version="1.0" charset="iso-8859-2" ?><!DOCTYPE html><p>` + string([]rune{225}) + "</p></html>",
	} {
		subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
		b, err := ioutil.ReadAll(fixXMLCharset(subCtx, strings.NewReader(elt)))
		subCancel()
		if err != nil {
			t.Errorf("%d. read: %v", i, err)
			continue
		}
		got := string(b)
		if got != want {
			t.Errorf("%d. got %q, want %q.", i, got, want)
		}
	}
}
