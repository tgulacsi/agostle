// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
)

func TestTextToHTML(t *testing.T) {
	var buf bytes.Buffer
	r := textToHTML(strings.NewReader("árvíztűrő <em>tükörfúrógép</em>"))
	if _, err := io.Copy(&buf, r); err != nil {
		t.Errorf("read: %v", err)
	}
	const wanted = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body><pre>árvíztűrő &lt;em&gt;tükörfúrógép&lt;/em&gt;
</pre></body></html>`
	if !bytes.Equal(buf.Bytes(), []byte(wanted)) {
		t.Errorf("mismatch:\n\tgot\n%s\n\twanted\n%s", buf.String(), wanted)
	}
}

func setTestLogger(t *testing.T) func() {
	SetLogger(testr.New(t))
	return func() { SetLogger(logr.Discard()) }
}

var testDir string

func TestMain(m *testing.M) {
	var err error
	testDir, err = os.MkdirTemp("", "agostle-test-")
	if err != nil {
		fmt.Println(err)
		os.Exit(13)
	}
	*ConfWorkdir = testDir
	_ = LoadConfig(context.Background(), "")
	code := m.Run()
	_ = os.RemoveAll(testDir)
	os.Exit(code)
}
