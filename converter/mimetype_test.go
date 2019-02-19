// Copyright 2019 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"encoding/base64"
	"testing"
)

func BenchmarkMIMEDetector(t *testing.B) {
	b, err := base64.StdEncoding.DecodeString(testDocx)
	if err != nil {
		t.Fatal(err)
	}
	seq := MultiMIMEDetector{Detectors: []MIMEDetector{H2nonMIMEDetector{}, HTTPMIMEDetector{}, VasileMIMEDetector{}}}
	const want = testDocxMIME
	if got, err := seq.Match(b); err != nil {
		t.Fatal(err)
	} else if got != want {
		if got != "application/zip" {
			t.Fatalf("got %s, wanted %s", got, want)
		}
		t.Logf("got %s, wanted %s", got, want)
	}

	t.Run("Sequential", func(t *testing.B) {
		for i := 0; i < t.N; i++ {
			seq.Match(b)
		}
	})

	par := seq
	par.Parallel = true
	t.Run("Parallel", func(t *testing.B) {
		for i := 0; i < t.N; i++ {
			par.Match(b)
		}
	})
}

const testDocxMIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
const testDocx = `UEsDBBQABgAIAAAAIQA/d5Jo+AEAAAcNAAATAAgCW0NvbnRlbnRfVHlwZXNdLnhtbCCiBAIooAAC
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAADM
l11P2zAUhu+R9h8i306NC0wTmppysY/LgTQm7da1T1pv/pJ9CvTfz47bCLGUBEImbio19vu+z7Gt
+GRxea9VcQs+SGsqclrOSQGGWyHNuiI/b77NLkgRkBnBlDVQkR0Ecrl8d7K42TkIRVSbUJENovtE
aeAb0CyU1oGJI7X1mmH869fUMf6HrYGezecfKbcGweAMkwdZLr5AzbYKi6/38XEm+e1gTYrPeWLK
qojUyaAZoEc0z5Y40y1Jz7sVHlR4JGHOKckZxnF6a8Sj8mf70suobOaEjXThfZxwJAFlXXdCNQPd
muR2HGqfdRX32UsBxTXz+J3pOIveWS+osHyro7J82qajNlvXkkOrT27OWw4hxAOkVdmOaCbNoeYu
Dr4NaPUvrahE0NfeunA6Gqc1TX7gUUK77gMZzt4Aw/kbYPjwvxmac2m2egU+nqTXP5itdS9EwJ2C
8PoE2bc/HhCjYAqAvXMvwh2sfkxG8cC8F6S2Fo3FKXajte6FACMmYjg49yJsgAnw49+P/xBk44H5
49+NL81PmzVJ/dl4YP4E9Q/Mz8s0/l4Yt/4T5A9e/5jHVgqmINhb90Jg7HUh/44/iY3NU5FxZg==`
