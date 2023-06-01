// Copyright 2023 Tamás Gulácsi. All rights reserved.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"

	kithttp "github.com/go-kit/kit/transport/http"
)

var stemConvertServer = kithttp.NewServer(
	stemConvertEP,
	stemConvertDecode,
	stemConvertEncode,
	kithttp.ServerBefore(defaultBeforeFuncs...),
	kithttp.ServerAfter(kithttp.SetContentType("application/json")),
)

var errNotImplemented = errors.New("not implemented")

func stemConvertEP(ctx context.Context, request interface{}) (response interface{}, err error) {
	return nil, errNotImplemented
}
func stemConvertDecode(ctx context.Context, r *http.Request) (interface{}, error) {
	return nil, errNotImplemented
}
func stemConvertEncode(ctx context.Context, w http.ResponseWriter, response interface{}) error {
	return errNotImplemented
}

// stem the words in the io.Reader, using the given language, using hunspell.
func stem(ctx context.Context, r io.Reader, language string) ([][2]string, error) {
	if language == "" {
		return nil, errors.New("language must be set")
	}
	var buf, errBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, "hunspell", "-d", language, "-s")
	cmd.Env = append(os.Environ(), "LANG="+language+".UTF-8")
	cmd.Stdin = r
	cmd.Stdout = &buf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%v (%s): %w", cmd.Args, errBuf.String(), err)
	}
	var res [][2]string
	for _, line := range bytes.Split(buf.Bytes(), []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		pre, post, found := bytes.Cut(line, []byte{' '})
		if !found {
			res = append(res, [2]string{string(line), ""})
			continue
		}
		res = append(res, [2]string{string(pre), string(post)})
	}
	return res, nil
}
