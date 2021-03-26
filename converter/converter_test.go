// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bytes"
	"io"
	"strings"
	"testing"
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
