// Copyright 2013 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"context"

	"github.com/spf13/cobra"
	"github.com/tgulacsi/agostle/converter"
)

func init() {

	pdfCmd := &cobra.Command{
		Use: "pdf",
	}
	agostleCmd.AddCommand(pdfCmd)

	Log := logger.Log
	var out string
	{
		var sort bool
		mergeCmd := &cobra.Command{
			Use:   "merge",
			Short: "merges the given PDFs into one",
			Long: `Usage:
	[globalopts] pdf_merge [-sort] [-o=/dest/merged.pdf] lot of pdf files

Example:
	-o=/dest/merged.pdf *.pdf", "-o=/dest/merged.pdf -split 2.pdf 1.pdf
`,
			Aliases: []string{"pdf_merge"},
			Run: func(cmd *cobra.Command, args []string) {
				if err := mergePdf(out, args, sort); err != nil {
					Log("msg", "mergePdf", "out", out, "sort", sort, "args", args, "error", err)
					os.Exit(1)
				}
			},
		}
		mergeCmd.Flags().StringVarP(&out, "out", "o", "", "output file")
		mergeCmd.Flags().BoolVar(&sort, "sort", false, "shall we sort the files by name before merge?")
		agostleCmd.AddCommand(mergeCmd)
		pdfCmd.AddCommand(mergeCmd)
	}

	splitCmd := &cobra.Command{
		Use:     "split",
		Short:   "splits the given PDF into one per page",
		Aliases: []string{"pdf_split"},
		Run: func(cmd *cobra.Command, args []string) {
			fn := inpFromArgs(args)
			if err := splitPdfZip(out, fn); err != nil {
				Log("msg", "splitPdfZip", "out", out, "fn", fn, "error", err)
				os.Exit(1)
			}
		},
	}
	splitCmd.Flags().StringVarP(&out, "out", "o", "", "output file")
	agostleCmd.AddCommand(splitCmd)
	pdfCmd.AddCommand(splitCmd)

	countCmd := &cobra.Command{
		Use:     "count",
		Short:   "prints the number of pages in the given pdf",
		Aliases: []string{"pdf_count"},
		Long:    `[globalopts] pdf_count multipage.pdf`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := countPdf(args[0]); err != nil {
				Log("msg", "countPdf", "fn", args[0], "error", err)
				os.Exit(1)
			}
		},
	}
	agostleCmd.AddCommand(countCmd)
	pdfCmd.AddCommand(countCmd)

	cleanCmd := &cobra.Command{
		Use:     "clean",
		Aliases: []string{"pdf_clean"},
		Short:   "clean PDF from encryption",
		Long:    `Usage: [-o=/dest/file.pdf] dirty.pdf`,
		Run: func(cmd *cobra.Command, args []string) {
			fn := inpFromArgs(args)
			if err := cleanPdf(out, fn); err != nil {
				Log("msg", "cleanPdf", "out", out, "fn", fn, "error", err)
				os.Exit(1)
			}
		},
	}
	cleanCmd.Flags().StringVarP(&out, "out", "o", "-", "output filename")
	agostleCmd.AddCommand(cleanCmd)
	pdfCmd.AddCommand(cleanCmd)

	{
		var mime string
		topdfCmd := &cobra.Command{
			Use:   "topdf",
			Short: "tries to convert the given file (you can specify its mime-type) to PDF",
			Long:  `[globalopts] [-mime=input-mime/type] [-o=output.pdf] input.something`,
			Run: func(cmd *cobra.Command, args []string) {
				fn := inpFromArgs(args)
				if err := toPdf(out, fn, mime); err != nil {
					Log("msg", "topdf", "out", out, "fn", fn, "mime", mime, "error", err)
					os.Exit(1)
				}
			},
		}
		topdfCmd.Flags().StringVarP(&out, "out", "o", "-", "output file")
		topdfCmd.Flags().StringVar(&mime, "mime", "application/octet-stream", "input mimetype")
		agostleCmd.AddCommand(topdfCmd)
	}

	fillPdfCmd := &cobra.Command{
		Use:     "fill [-o output] input.pdf key1=value1 key2=value2...",
		Short:   "fill PDF form",
		Aliases: []string{"pdf_fill", "fill_form", "pdf_fill_form"},
		Run: func(cmd *cobra.Command, args []string) {
			if err := fillFdf(out, args[0], args[1:]...); err != nil {
				Log("msg", "fillPdf", "out", out, "args", args, "error", err)
				os.Exit(1)
			}
		},
	}
	fillPdfCmd.Flags().StringVarP(&out, "out", "o", "-", "output file")
	agostleCmd.AddCommand(fillPdfCmd)
	pdfCmd.AddCommand(fillPdfCmd)
}

func splitPdfZip(outfn, inpfn string) error {
	var changed bool
	if inpfn, changed = ensureFilename(inpfn, false); changed {
		defer func() { _ = os.Remove(inpfn) }()
	}
	filenames, err := converter.PdfSplit(inpfn)
	if err != nil {
		return err
	}
	outfh, err := openOut(outfn)
	if err != nil {
		return err
	}
	files := make([]converter.ArchFileItem, len(filenames))
	for i, nm := range filenames {
		files[i] = converter.ArchFileItem{Filename: nm}
	}
	ze := converter.ZipFiles(outfh, false, false, files...)
	closeErr := outfh.Close()
	if ze != nil {
		return ze
	}
	return closeErr
}

func mergePdf(outfn string, inpfn []string, sortFiles bool) error {
	if sortFiles {
		sort.Strings(inpfn)
	}
	return converter.PdfMerge(context.Background(), outfn, inpfn...)
}

func cleanPdf(outfn, inpfn string) error {
	var changed bool
	fmt.Fprintf(os.Stderr, "inpfn=%s outfn=%s\n", inpfn, outfn)
	if inpfn, changed = ensureFilename(inpfn, false); changed {
		defer func() { _ = os.Remove(inpfn) }()
	}
	outfn, changed = ensureFilename(outfn, true)
	fmt.Fprintf(os.Stderr, "inpfn=%s outfn=%s\n", inpfn, outfn)
	if err := converter.PdfRewrite(outfn, inpfn); err != nil {
		if changed {
			_ = os.Remove(outfn)
		}
		return err
	}
	fh, err := os.Open(outfn)
	if err != nil {
		return err
	}
	_, err = io.Copy(os.Stdout, fh)
	_ = fh.Close()
	return err
}

func toPdf(outfn, inpfn string, mime string) error {
	return errors.New("not implemented")
}

func countPdf(inpfn string) error {
	n, err := converter.PdfPageNum(inpfn)
	if err != nil {
		return err
	}
	fmt.Printf("%d\n", n)
	return nil
}

func fillFdf(outfn, inpfn string, kv ...string) error {
	values := make(map[string]string, len(kv))
	for _, txt := range kv {
		i := strings.IndexByte(txt, '=')
		if i < 0 {
			logger.Log("no = in key=value arg!")
			continue
		}
		values[txt[:i]] = txt[i+1:]
	}
	return converter.PdfFillFdf(outfn, inpfn, values)
}
