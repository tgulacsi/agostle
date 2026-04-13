// Copyright 2017, 2026 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"strings"

	"context"

	"github.com/peterbourgon/ff/v4"
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
	logger := logger.With("fn", "outlookToEmail")
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
			logger.Info("Close")
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
	withOutFlag := func(name string) *ff.FlagSet {
		fs := ff.NewFlagSet(name)
		fs.StringVar(&out, 'o', "out", "", "output file")
		return fs
	}
	{
		var (
			split         bool
			outimg, pageS string
			imgsize       = "640x640"
		)
		fs := withOutFlag("mail")
		fs.BoolVar(&split, 0, "split", "split PDF to pages")
		fs.BoolVarDefault(&converter.SaveOriginalHTML, 0, "save-original-html", converter.SaveOriginalHTML, "save original html")
		fs.StringVar(&outimg, 0, "outimg", "", "output image format")
		fs.StringVar(&imgsize, 0, "imgsize", imgsize, "image size")
		fs.StringVar(&pageS, 0, "pages", "", "pages (comma separated)")
		mailToPdfZipCmd := ff.Command{Name: "mail", Flags: fs,
			ShortHelp: "convert mail to zip of PDFs",
			Usage:     "mail [-split] [-outimg=image/gif] [-imgsize=640x640] mailfile.eml",
			LongHelp: `reads a message/rfc822 email, converts all of it to PDF files
(including attachments), and outputs a zip file containing these pdfs,
optionally splits the PDFs to separate pages, and converts these pages to images.

Usage:
	mail2pdfzip [-split] [-outimg=image/gif] [-imgsize=640x640] mailfile.eml

Examples:
	mail2pdfzip -split --outimg=image/gif --imgsize=800x800 -o=/tmp/email.pdf.zip email.eml
`,
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 0 {
					inp = args[0]
				}
				pages := parseUint16s(strings.Split(pageS, ","))
				if outimg != "" && strings.IndexByte(outimg, '/') < 0 {
					outimg = "image/" + outimg
				}
				if err := mailToPdfZip(ctx, out, inp, split, outimg, imgsize, pages); err != nil {
					return fmt.Errorf("mailToPdfZip out=%s: %w", out, err)
				}
				return nil
			},
		}
		subcommands = append(subcommands, &mailToPdfZipCmd)
	}

	fs := withOutFlag("mail2tree")
	mailToTreeCmd := ff.Command{Name: "mail2tree", Flags: fs,
		ShortHelp: "extract mail tree to a directory",
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
	outlookToEmailCmd := ff.Command{Name: "outlook2email", Flags: fs,
		ShortHelp: "convert outlook .msg to standard .eml",
		LongHelp:  "uses libemail-outlook-message-perl if installed, or docker to install && run that script",
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
