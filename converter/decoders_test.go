// Copyright 2013 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bytes"
	"io/ioutil"
	"testing"
)

var deBorkTests = [][2]string{
	[2]string{"saddsgfgrv$#EWfedsv+dsfsadgfg", "saddsgfgrv$#EWfedsv+dsfsadgfg"},
	[2]string{`+ADw-p class+AD0AIg-MsoNormal+ACI- align+AD0AIg-right+ACI- style+AD0AIg-tex=
t-align:right+ACIAPgA8-span style+AD0AIg-font-size:11.0pt+ADs-font-family:+=
ACY-quot+ADs-Calibri+ACY-quot+ADs-,+ACY-quot+ADs-sans-serif+ACY-quot+ADsAOw=
-color:+ACM-1F497D+ACIAPgA8-img border+AD0AIg-0+ACI- width+AD0AIg-124+ACI- =
height+AD0AIg-26+ACI- id+AD0AIgBf-x0000+AF8-i1026+ACI- src+AD0AIg-cid:image=
002.png+AEA-01CE1E47.CD9AD230+ACI- alt+AD0AIg-Leiras: C:+AFw-Docume=
nts and Settings+AFw-holloa+AFw-Application Data+AFw-brands.png+ACIAPgA8-/s=
pan+AD4APA-span style+AD0AIg-color:+ACM-1F497D+ADs-mso-fareast-language:EN-=
US+ACIAPgA8-o:p+AD4APA-/o:p+AD4APA-/span+AD4APA-/p+AD4-`,
		`<p class="MsoNormal" align="right" style="text-align:right"><span style="font-size:11.0pt;font-family:&quot;Calibri&quot;,&quot;sans-serif&quot;;color:#1F497D"><img border="0" width="124" height="26" id="_x0000_i1026" src="cid:image002.png@01CE1E47.CD9AD230" alt="Leiras: C:\Documents and Settings\holloa\Application Data\brands.png"></span><span style="color:#1F497D;mso-fareast-language:EN-US"><o:p></o:p></span></p>`},
	[2]string{`+AF8AXwBfAF8AXwBfAF8AXwBfAF8AXwBfAF8AXwBfAF8AXwBfAF8AXwBfAF=
8AXwBfAF8AXwBfAF8AXwBfAF8AXwBfAF8AXwBfAF8AXwBfAF8AXwBfAF8AXwBfAF8AXwBfAF8AX=
wBfAF8AXwBfAF8AXwBfAF8AXwBfAF8AXwBfAF8AXw-`,
		`_________________________________________________________________`},
}

func TestB64QuoPriDecoder(t *testing.T) {
	for i, tf := range deBorkTests {
		r := NewB64QuoPriDecoder(bytes.NewReader([]byte(tf[0])))
		out, err := ioutil.ReadAll(r)
		if err != nil {
			t.Errorf("error with reading: %s", err)
		}
		out = bytes.Replace(out, []byte{0}, nil, -1)
		if string(out) != tf[1] {
			awaited := []byte(tf[1])
			d := findDiff(awaited, out)
			if d == -2 {
				t.Errorf("%d. length mismatch: awaited %d, got %d:\n\t%s\nvs.\n\t%s", i+1,
					len(awaited), len(out), awaited, out)
			} else {
				x := max(d-3, 0)
				y := min(d+3, len(out))
				t.Errorf("%d. awaited\n\t%s\ngot\n\t%s\n diff@%d: %v <> %v",
					i+1, tf[1], out, d, awaited[x:y], out[x:y])
			}
		}
	}
}

func findDiff(a, b []byte) int {
	if len(a) != len(b) {
		return -2
	}
	for i, x := range a {
		if x != b[i] {
			return i
		}
	}
	return -1
}

func TestEqsignStripper(t *testing.T) {
	data := []byte("abraka=dabraka=\r\nprix=\nprax=prux\nquix\r\npux")
	await := "abraka=dabrakaprixprax=prux\nquix\r\npux"
	read, err := ioutil.ReadAll(NewEqsignStripper(bytes.NewBuffer(data)))
	if err != nil {
		t.Errorf("error with stripper: %s", err)
	}
	if string(read) != await {
		t.Errorf("data mismatch @%d: \n\t%s [%d]!=[%d] \n\t%s",
			findDiff(read, []byte(await)), await, len([]byte(await)),
			len(read), string(read))
	}
}

func TestCidMapper(t *testing.T) {
	data := []byte("<html><body><a\nsrc=\"cid:<image.png@ewee>\"\n>b</a></body></html>")
	await := "<html><body><a\nsrc=\"images/image.png@ewee\"\n>b</a></body></html>"
	cids := make(map[string]string, 1)
	read, err := ioutil.ReadAll(NewCidMapper(cids, "images", bytes.NewBuffer(data)))
	if err != nil {
		t.Errorf("error with stripper: %s", err)
	}
	if string(read) != await {
		t.Errorf("data mismatch: awaited\n\t%s\n%v != got\n\t%s\n%v",
			await, []byte(await), string(read), read)
	}
	t.Logf("cids: %s", cids)
}
