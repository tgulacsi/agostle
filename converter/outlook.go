// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// NewOLEStorageReader converts Outlook .msg files to .eml RFC822 email files.
// For this it uses perl Email::Outlook::Message (thanks, @matijs), and returns
// an io.Reader with the converted data.
//
// This calls out to perl, and needs Email::Outlook::Message (can be installed
// with `cpan -i Email::Outlook::Message`).
//
// See http://www.matijs.net/software/msgconv
func NewOLEStorageReader(ctx context.Context, r io.Reader) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	tr := io.TeeReader(r, &buf)
	rc, err := newOLEStorageReaderDirect(ctx, tr)
	if err == nil {
		br := bufio.NewReader(rc)
		if _, err = br.Peek(1); err == nil {
			return struct {
				io.Reader
				io.Closer
			}{
				br,
				rc,
			}, nil
		}
	}
	if !strings.Contains(err.Error(), "Can't locate Email/Outlook/Message.pm in @INC") {
		return rc, fmt.Errorf("%s: %w", "Can't locate Email/Outlook/Message.pm in @INC", err)
	}
	rc.Close()
	logger.Info("Email::Outlook::Message is not installed, trying with docker")
	return newOLEStorageReaderDocker(ctx, io.MultiReader(bytes.NewReader(buf.Bytes()), r))
}

func newOLEStorageReaderDirect(ctx context.Context, r io.Reader) (io.ReadCloser, error) {
	var err error
	// Email::Outlook::Message needs a filename!
	var remove bool
	in, ok := r.(*os.File)
	if ok {
		defer in.Close()
	} else {
		in, err = os.CreateTemp("", ".msg")
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(in, r)
		closeErr := in.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, err
		}
		remove = true
	}

	var errBuf bytes.Buffer
	/*
		#!/usr/bin/perl -w
		#
		# msgdump.pl:
		#
		# Dump .MSG files (made by Outlook (Express)) as multipart MIME messages.
		#

		use Email::Outlook::Message;
		use vars qw($VERSION);
		$VERSION = "0.903";

		foreach my $file (@ARGV) {
		  print new Email::Outlook::Message($file, 0)->to_email_mime->as_string;
		}
	*/
	cmd := Exec.CommandContext(ctx, "perl", "-w",
		"-e", "use Email::Outlook::Message",
		"-e", "print (new Email::Outlook::Message($ARGV[0], 1)->to_email_mime->as_string);",
		"--", in.Name(),
	)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = &errBuf
	logger.Info("OLEStorageReader", "args", cmd.Args)
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("OLEStorageReader: %s: %w", errBuf.String(), err)
	}
	go func() {
		if err = cmd.Wait(); err != nil {
			errTxt := errBuf.String()
			logger.Info("WARN OLEStorageReader", "args", cmd.Args, "errTxt", errTxt, "error", err)
			err = fmt.Errorf("%s: %w", errTxt, err)
		}
		_ = pw.CloseWithError(err)
		if remove {
			_ = os.Remove(in.Name())
		}
	}()
	return struct {
		io.Reader
		io.Closer
	}{
		pr,
		pw,
	}, nil
}

func newOLEStorageReaderDocker(ctx context.Context, r io.Reader) (io.ReadCloser, error) {
	cmd := Exec.CommandContext(ctx, "docker", "build", "-t", "tgulacsi/agostle-outlook2email", "-")
	cmd.Stdin = strings.NewReader(`FROM debian:testing
MAINTAINER Tamás Gulácsi <tgulacsi78@gmail.com>

ENV DEBIAN_FRONTEND noninteractive
RUN apt-get -y update
#&& apt-get -y upgrade
RUN apt-get -y install libemail-outlook-message-perl

CMD ["/bin/sh", "-c", "cat ->/tmp/input.msg && perl -w -e 'use Email::Outlook::Message' -e 'print (new Email::Outlook::Message(\"/tmp/input.msg\", 1)->to_email_mime->as_string);' --"]
`)
	var errBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(&errBuf, os.Stderr)
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		logger.Info("ERROR docker build tgulacsi/agostle-outlook2email", "error", err, "errTxt", errBuf.String())
		return nil, fmt.Errorf("docker build: %w", err)
	}
	cmd = Exec.CommandContext(ctx, "docker", "run", "-i", "tgulacsi/agostle-outlook2email")
	cmd.Stdin = r
	errBuf.Reset()
	cmd.Stderr = io.MultiWriter(&errBuf, os.Stderr)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	if err := cmd.Start(); err != nil {
		logger.Info("ERROR docker run -i tgulacsi/agostle-outlook2email", "error", err)
		return nil, fmt.Errorf("docker run: %w", err)
	}
	go func() {
		var err error
		if err = cmd.Wait(); err != nil {
			errTxt := errBuf.String()
			logger.Info("OLEStorageReader", "args", cmd.Args, "stderr", errTxt, "error", err)
			err = fmt.Errorf("%s: %w", errTxt, err)
		}
		_ = pw.CloseWithError(err)
		//if remove {
		//os.Remove(in.Name())
		//}
	}()
	return struct {
		io.Reader
		io.Closer
	}{
		pr,
		pw,
	}, nil
}
