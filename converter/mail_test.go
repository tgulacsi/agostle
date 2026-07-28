// Copyright 2017, 2026 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"net/mail"
	"strconv"
	"testing"

	"github.com/tgulacsi/go/i18nmail"
)

func TestMailAddress(t *testing.T) {
	for i, str := range []struct {
		Encoded, Name, Address string
	}{
		{"=?iso-8859-2?Q?Bogl=E1rka_Tak=E1cs?= <tbogi77@gmail.com>",
			"Boglárka Takács", "tbogi77@gmail.com"},
		{"=?iso-8859-2?Q?Claim_Division_=28claim=40example=2Ecom=29?= <claim@example.com>",
			"Claim Division (claim@example.com)", "claim@example.com"},
		{`=?utf-7?B?SytBTlktQkUgTmVtemV0aytBUFktemkgKG5lbXpldGtvemkrQUVBLWtvYmUuaHUp?= <nemzetkozi@kobe.hu>`, "KÖBE Nemzetközi (nemzetkozi@kobe.hu)", "nemzetkozi@kobe.hu"},
	} {
		check := func(name, address string) {
			if got, want := address, str.Address; got != want {
				t.Errorf("got Address=%s, wanted %s", got, want)
			} else if got, want := name, str.Name; got != want {
				t.Errorf("got Name=%s, wanted %s", got, want)
			}
		}

		mh := make(map[string][]string, 1)
		k := strconv.Itoa(i+1) + "."
		mh[k] = []string{str.Encoded}
		mailHeader := mail.Header(mh)
		if addr, err := mailHeader.AddressList(k); err == nil {
			t.Logf("address for %s: %q", k, addr)
			check(addr[0].Name, addr[0].Address)
		} else {
			t.Logf("error parsing address %s(%q) with std mail.Header: %s", k, mailHeader.Get(k), err)
		}
		if addr, err := i18nmail.ParseAddress(str.Encoded); err != nil {
			t.Errorf("error parsing address %q: %s", str.Encoded, err)
		} else {
			t.Logf("address for %q: %q <%s>", str.Encoded, addr.Name, addr.Address)
			check(addr.Name, addr.Address)
		}
	}
}

func TestHeaderGetFileName(t *testing.T) {
	for tN, tc := range []struct {
		Header map[string][]string
		Want   string
	}{
		{
			map[string][]string{"Content-Description": {"20160519_1211_GKIU1AM.docx"}, "Content-Disposition": {"attachment; filename=\"20160519_1211_GKIU1AM.docx\"; size=271904; creation-date=\"Thu, 19 May 2016 10:15:00 GMT\"; modification-date=\"Thu, 19 May 2016 10:15:00 GMT\""}, "Content-Id": {"<59DA1D23CFE5BB419EE50F7DF8CE0CAC@example.com>"}, "Content-Transfer-Encoding": {"base64"}, "X-Hashoffullmessage": {"2WupAYCPqc_630rRYxms6hyEkgk="}, "X-Filename": {"20160519_1211_GKIU1AM.docx"}, "Content-Type": {"application/octet-stream; name=\"20160519_1211_GKIU1AM.docx\""}},
			"20160519_1211_GKIU1AM.docx",
		},
	} {
		if got := headerGetFileName(tc.Header); got != tc.Want {
			t.Errorf("%d. got %q, wanted %q (%q).", tN, got, tc.Want, tc.Header)
		}
	}
}
