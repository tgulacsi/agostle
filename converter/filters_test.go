// Copyright 2013 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"
)

func TestFixXMLHeader(t *testing.T) {
	want := `<?xml version="1.0" charset="utf-8"?><!DOCTYPE html></html>`
	for i, elt := range []string{
		want,
		strings.Replace(want, "charset", "encoding", 1),
	} {
		b, err := ioutil.ReadAll(fixXMLHeader(strings.NewReader(elt)))
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
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		b, err := ioutil.ReadAll(fixXMLCharset(ctx, strings.NewReader(elt)))
		cancel()
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
