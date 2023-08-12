// Copyright 2023 Tamás Gulácsi. All rights reserved.
//
// SPDX-License-Identifier: Apache-2.0

package converter

import (
	"context"
	"crypto/rand"

	"github.com/oklog/ulid/v2"
)

type ctxReqID struct{}

func SetRequestID(ctx context.Context, reqID string) context.Context {
	if v, ok := ctx.Value(ctxReqID{}).(string); ok && v != "" {
		return ctx
	}
	if reqID == "" {
		reqID = NewULID().String()
	}
	return context.WithValue(ctx, ctxReqID{}, reqID)
}
func GetRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxReqID{}).(string); ok && v != "" {
		return v
	}
	return NewULID().String()
}

func NewULID() ulid.ULID {
	return ulid.MustNew(ulid.Now(), rand.Reader)
}
