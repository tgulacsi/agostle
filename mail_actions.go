// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"context"

	"github.com/go-kit/kit/log"
	"github.com/tgulacsi/agostle/converter"
	"gopkg.in/alecthomas/kingpin.v2"
)

func mailToPdfZip(ctx context.Context, outfn, inpfn string, splitted bool, outimg string, imgsize string) error {
	input, err := openIn(inpfn)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	if !splitted && outimg == "" {
		return converter.MailToPdfZip(ctx, outfn, input, "message/rfc822")
	}
	return converter.MailToSplittedPdfZip(ctx, outfn, input, "message/rfc822",
		splitted, outimg, imgsize)
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
	Log := log.With(logger, "fn", "outlookToEmail").Log
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
			Log("msg", "Close", "error", err)
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
	var inp, out string
	withOutFlag := func(cmd *kingpin.CmdClause) {
		cmd.Flag("out", "output file").Short('o').StringVar(&out)
		cmd.Arg("inp", "input file").Default("-").StringVar(&inp)
	}
	{
		var (
			split   bool
			outimg  string
			imgsize = "640x640"
		)
		mailToPdfZipCmd := app.Command("mail", `convert mail to zip of PDFs

reads a message/rfc822 email, converts all of it to PDF files
(including attachments), and outputs a zip file containing these pdfs,
optionally splits the PDFs to separate pages, and converts these pages to images.

Usage:
	mail2pdfzip [-split] [-outimg=image/gif] [-imgsize=640x640] mailfile.eml

Examples:
	mail2pdfzip -split --outimg=image/gif --imgsize=800x800 -o=/tmp/email.pdf.zip email.eml
`).Alias("mail2pdfzip").Alias("mailToPdfZip")

		withOutFlag(mailToPdfZipCmd)
		mailToPdfZipCmd.Flag("split", "split PDF to pages").BoolVar(&split)
		mailToPdfZipCmd.Flag("save-original-html", "save original html").Default(strconv.FormatBool(converter.SaveOriginalHTML)).BoolVar(&converter.SaveOriginalHTML)
		mailToPdfZipCmd.Flag("outimg", "output image format").StringVar(&outimg)
		mailToPdfZipCmd.Flag("imgsize", "image size").Default("640x480").StringVar(&imgsize)
		commands[mailToPdfZipCmd.FullCommand()] = func(ctx context.Context) error {
			if err := mailToPdfZip(ctx, out, inp, split, outimg, imgsize); err != nil {
				return fmt.Errorf("mailToPdfZip out=%s: %w", out, err)
			}
			return nil
		}
	}

	mailToTreeCmd := app.Command("mail2tree", "extract mail tree to a directory")
	withOutFlag(mailToTreeCmd)
	commands[mailToTreeCmd.FullCommand()] = func(ctx context.Context) error {
		if err := mailToTree(ctx, out, inp); err != nil {
			return fmt.Errorf("mailToTree out=%s: %w", out, err)
		}
		return nil
	}

	outlookToEmailCmd := app.Command("outlook2email", `convert outlook .msg to standard .eml

uses libemail-outlook-message-perl if installed, or docker to install && run that script`).Alias("msg2eml")
	withOutFlag(outlookToEmailCmd)
	commands[outlookToEmailCmd.FullCommand()] = func(ctx context.Context) error {
		if err := outlookToEmail(ctx, out, inp); err != nil {
			return fmt.Errorf("outlookToEmail out=%s: %w", out, err)
		}
		return nil
	}
}
