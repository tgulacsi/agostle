// Copyright 2013 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

// Needed: /email/convert?splitted=1&errors=1&id=xxx Accept: images/gif
//  /pdf/merge Accept: application/zip

import (
	"io"
	"net/http"

	"context"

	"github.com/tgulacsi/agostle/converter"

	kithttp "github.com/go-kit/kit/transport/http"
)

var outlookToEmailServer = kithttp.NewServer(
	outlookToEmailEP,
	outlookToEmailDecode,
	outlookToEmailEncode,
	kithttp.ServerBefore(defaultBeforeFuncs...),
	kithttp.ServerAfter(kithttp.SetContentType("mail/rfc822")),
)

func outlookToEmailDecode(ctx context.Context, r *http.Request) (interface{}, error) {
	return getOneRequestFile(ctx, r)
}

func outlookToEmailEP(ctx context.Context, request interface{}) (response interface{}, err error) {
	f := request.(reqFile)
	defer func() { _ = f.Close() }()
	return converter.NewOLEStorageReader(f)
}

func outlookToEmailEncode(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	res := response.(io.ReadCloser)
	defer func() { _ = res.Close() }()
	_, err := io.Copy(w, res)
	return err
}
