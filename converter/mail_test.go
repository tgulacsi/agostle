// Copyright 2013 The Agostle Authors. All rights reserved.
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
	for i, str := range [][3]string{
		[3]string{"=?iso-8859-2?Q?Bogl=E1rka_Tak=E1cs?= <tbogi77@gmail.com>",
			"Boglárka Takács", "<tbogi77@gmail.com>"},
		[3]string{"=?iso-8859-2?Q?Claim_Divison_=28claim=40example=2Ecom=29?= <claim@example.com>",
			"Claim_Division claim@example.com", "<claim@example.com>"},
	} {
		mh := make(map[string][]string, 1)
		k := strconv.Itoa(i)
		mh[k] = []string{str[0]}
		mailHeader := mail.Header(mh)
		if addr, err := mailHeader.AddressList(k); err == nil {
			t.Errorf("address for %s: %q", k, addr)
		} else {
			t.Logf("error parsing address %s(%q): %s", k, mailHeader.Get(k), err)
		}
		if addr, err := i18nmail.ParseAddress(str[0]); err != nil {
			t.Errorf("error parsing address %q: %s", str[0], err)
		} else {
			t.Logf("address for %q: %q <%s>", str[0], addr.Name, addr.Address)
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
