// Copyright 2017, 2022 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"context"

	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/tgulacsi/agostle/converter"
)

func mailToPdfZip(ctx context.Context, outfn, inpfn string, splitted bool, outimg string, imgsize string, pages []uint16) error {
	input, err := openIn(inpfn)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	splitted = splitted || len(pages) != 0
	if !splitted && outimg == "" {
		return converter.MailToPdfZip(ctx, outfn, input, "message/rfc822")
	}
	return converter.MailToSplittedPdfZip(ctx, outfn, input, "message/rfc822",
		splitted, outimg, imgsize, pages)
}

func mailToTree(ctx context.Context, outdir, inpfn string) error {
	input, err := openIn(inpfn)
	if err != nil {
		return err
	}
	if strings.HasSuffix(outdir, ".zip") && !isDir(outdir) {
		return converter.MailToZip(ctx, outdir, input, "message/rfc822")
	}
	return converter.MailToTree(ctx, outdir, input)
}

func outlookToEmail(ctx context.Context, outfn, inpfn string) error {
	logger := logger.WithValues("fn", "outlookToEmail")
	inp, err := openIn(inpfn)
	if err != nil {
		return err
	}
	out, err := openOut(outfn)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			logger.Info("msg", "Close")
			if err == nil {
				err = closeErr
			}
		}
	}()
	var r io.ReadCloser
	r, err = converter.NewOLEStorageReader(ctx, inp)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, r)
	_ = r.Close()
	return err
}

func init() {
	inp := "-"
	var out string
	withOutFlag := func(name string) *flag.FlagSet {
		fs := newFlagSet(name)
		fs.StringVar(&out, "o", "", "output file")
		return fs
	}
	{
		var (
			split         bool
			outimg, pageS string
			imgsize       = "640x640"
		)
		fs := withOutFlag("mail")
		fs.BoolVar(&split, "split", false, "split PDF to pages")
		fs.BoolVar(&converter.SaveOriginalHTML, "save-original-html", converter.SaveOriginalHTML, "save original html")
		fs.StringVar(&outimg, "outimg", "", "output image format")
		fs.StringVar(&imgsize, "imgsize", imgsize, "image size")
		fs.StringVar(&pageS, "pages", "", "pages (comma separated)")
		mailToPdfZipCmd := ffcli.Command{Name: "mail", ShortHelp: "convert mail to zip of PDFs",
			ShortUsage: "mail [-split] [-outimg=image/gif] [-imgsize=640x640] mailfile.eml",
			LongHelp: `reads a message/rfc822 email, converts all of it to PDF files
(including attachments), and outputs a zip file containing these pdfs,
optionally splits the PDFs to separate pages, and converts these pages to images.

Usage:
	mail2pdfzip [-split] [-outimg=image/gif] [-imgsize=640x640] mailfile.eml

Examples:
	mail2pdfzip -split --outimg=image/gif --imgsize=800x800 -o=/tmp/email.pdf.zip email.eml
`,
			FlagSet: fs,
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 0 {
					inp = args[0]
				}
				pages := parseUint16s(strings.Split(pageS, ","))
				if err := mailToPdfZip(ctx, out, inp, split, outimg, imgsize, pages); err != nil {
					return fmt.Errorf("mailToPdfZip out=%s: %w", out, err)
				}
				return nil
			},
		}
		subcommands = append(subcommands, &mailToPdfZipCmd)
	}

	fs := withOutFlag("mail2tree")
	mailToTreeCmd := ffcli.Command{Name: "mail2tree", ShortHelp: "extract mail tree to a directory",
		FlagSet: fs,
		Exec: func(ctx context.Context, args []string) error {
			if len(args) != 0 {
				inp = args[0]
			}
			if err := mailToTree(ctx, out, inp); err != nil {
				return fmt.Errorf("mailToTree out=%s: %w", out, err)
			}
			return nil
		},
	}
	subcommands = append(subcommands, &mailToTreeCmd)

	fs = withOutFlag("outlook2email")
	outlookToEmailCmd := ffcli.Command{Name: "outlook2email", ShortHelp: "convert outlook .msg to standard .eml",
		LongHelp: "uses libemail-outlook-message-perl if installed, or docker to install && run that script",
		FlagSet:  fs,
		Exec: func(ctx context.Context, args []string) error {
			if len(args) != 0 {
				inp = args[0]
			}
			if err := outlookToEmail(ctx, out, inp); err != nil {
				return fmt.Errorf("outlookToEmail out=%s: %w", out, err)
			}
			return nil
		},
	}
	subcommands = append(subcommands, &outlookToEmailCmd)
}
