// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"io"
	"os"
	"strings"

	"context"

	"github.com/go-kit/kit/log"
	"github.com/spf13/cobra"
	"github.com/tgulacsi/agostle/converter"
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
	Log := logger.Log
	var out string
	{
		var (
			split   bool
			outimg  string
			imgsize = "640x640"
		)
		mailToPdfZipCmd := &cobra.Command{
			Use:   "mail",
			Short: "convert mail to zip of PDFs",
			Long: `reads a message/rfc822 email, converts all of it to PDF files
(including attachments), and outputs a zip file containing these pdfs,
optionally splits the PDFs to separate pages, and converts these pages to images.

Usage:
	mail2pdfzip [-split] [-outimg=image/gif] [-imgsize=640x640] mailfile.eml

Examples:
	mail2pdfzip -split --outimg=image/gif --imgsize=800x800 -o=/tmp/email.pdf.zip email.eml
`,
			Aliases: []string{"mail2pdfzip", "mail", "mailToPdfZip"},
			Run: func(cmd *cobra.Command, args []string) {
				fn := inpFromArgs(args)
				if err := mailToPdfZip(ctx, out, fn, split, outimg, imgsize); err != nil {
					Log("msg", "mailToPdfZip to", "out", out, "error", err)
					os.Exit(1)
				}
			},
		}
		f := mailToPdfZipCmd.Flags()
		f.StringVarP(&out, "out", "o", "", "output file")
		f.BoolVar(&split, "split", false, "split PDF to pages")
		f.BoolVar(&converter.SaveOriginalHTML, "save-original-html", converter.SaveOriginalHTML, "save original html")
		f.StringVar(&outimg, "outimg", "", "output image format")
		f.StringVar(&imgsize, "imgsize", "640x640", "image size")
		agostleCmd.AddCommand(mailToPdfZipCmd)
	}

	mailToTreeCmd := &cobra.Command{
		Use:   "mail2tree",
		Short: "extract mail tree to a directory",
		Run: func(cmd *cobra.Command, args []string) {
			fn := inpFromArgs(args)
			if err := mailToTree(ctx, out, fn); err != nil {
				Log("msg", "mailToTree", "out", out, "fn", fn, "error", err)
				os.Exit(1)
				os.Exit(1)
			}
		},
	}
	mailToTreeCmd.Flags().StringVarP(&out, "out", "o", "", "output file")
	agostleCmd.AddCommand(mailToTreeCmd)

	outlookToEmailCmd := &cobra.Command{
		Use:     "outlook2email",
		Short:   "convert outlook .msg to standard .eml",
		Long:    "uses libemail-outlook-message-perl if installed, or docker to install && run that script",
		Aliases: []string{"msg2eml"},
		Run: func(cmd *cobra.Command, args []string) {
			fn := inpFromArgs(args)
			if err := outlookToEmail(ctx, out, fn); err != nil {
				Log("msg", "outlookToEmail", "out", out, "fn", fn, "error", err)
				os.Exit(1)
				os.Exit(1)
			}
		},
	}
	outlookToEmailCmd.Flags().StringVarP(&out, "out", "o", "", "output file")
	agostleCmd.AddCommand(outlookToEmailCmd)
}

func inpFromArgs(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return "-"
}
