// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"context"

	"github.com/pkg/errors"
	"github.com/tgulacsi/agostle/converter"
	"gopkg.in/alecthomas/kingpin.v2"
)

func init() {

	pdfCmd := app.Command("pdf", "pdf commands")

	var out string
	withOutFlag := func(cmd *kingpin.CmdClause) {
		cmd.Flag("out", "output file").Short('o').StringVar(&out)
	}
	{
		var sort bool
		mergeCmd := pdfCmd.Command("merge", "merges the given PDFs into one").
			Alias("pdf_merge").Alias("pdfmerge")
		withOutFlag(mergeCmd)
		mergeCmd.Flag("sort", "shall we sort the files by name before merge?").BoolVar(&sort)
		mergeInp := mergeCmd.Arg("inp", "input files").Strings()
		commands[mergeCmd.FullCommand()] = func(ctx context.Context) error {
			for i, s := range *mergeInp {
				if s == "" {
					(*mergeInp)[i] = "-"
				}
			}
			return errors.WithMessage(
				mergePdf(out, *mergeInp, sort),
				fmt.Sprintf("mergePDF out=%q sort=%v inp=%q", out, sort, mergeInp))
		}
	}

	splitCmd := pdfCmd.Command("split", "splits the given PDF into one per page").
		Alias("pdf_split")
	withOutFlag(splitCmd)
	splitInp := splitCmd.Arg("inp", "input file").Default("-").String()
	commands[splitCmd.FullCommand()] = func(ctx context.Context) error {
		if *splitInp == "" {
			*splitInp = "-"
		}
		return errors.WithMessage(
			splitPdfZip(ctx, out, *splitInp),
			fmt.Sprintf("splitPdfZip out=%q inp=%q", out, *splitInp))
	}

	countCmd := pdfCmd.Command("count", "prints the number of pages in the given pdf").
		Alias("pdf_count").Alias("pagecount").Alias("pageno")
	countInp := countCmd.Arg("inp", "input file").String()
	commands[countCmd.FullCommand()] = func(ctx context.Context) error {
		return errors.WithMessage(
			countPdf(ctx, *countInp),
			"countPdf inp="+*countInp)
	}

	cleanCmd := pdfCmd.Command("clean", "clean PDF from encryption").
		Alias("pdf_clean")
	withOutFlag(cleanCmd)
	cleanInp := cleanCmd.Arg("inp", "input file").String()
	commands[countCmd.FullCommand()] = func(ctx context.Context) error {
		return errors.WithMessage(
			cleanPdf(ctx, out, *cleanInp),
			fmt.Sprintf("cleanPdf out=%q inp=%q", out, *cleanInp))
	}

	{
		var mime string
		topdfCmd := pdfCmd.Command("topdf", "tries to convert the given file (you can specify its mime-type) to PDF")
		withOutFlag(topdfCmd)
		topdfCmd.Flag("mime", "input mimetype").Default("application/octet-stream").StringVar(&mime)
		topdfInp := topdfCmd.Arg("inp", "input file").String()
		commands[topdfCmd.FullCommand()] = func(ctx context.Context) error {
			return errors.WithMessage(
				toPdf(out, *topdfInp, mime),
				fmt.Sprintf("topdf out=%q inp=%q mime=%q", out, *topdfInp, mime))
		}
	}

	fillPdfCmd := pdfCmd.Command("fill", `fill PDF form
input.pdf key1=value1 key2=value2...`).
		Alias("pdf_fill").Alias("fill_form").Alias("pdf_fill_form")
	withOutFlag(fillPdfCmd)
	fillInp := fillPdfCmd.Arg("inp", "input file").String()
	fillKeyvals := fillPdfCmd.Arg("keyvals", "key1=val1, key2=val2...").Strings()
	commands[fillPdfCmd.FullCommand()] = func(ctx context.Context) error {
		return errors.WithMessage(
			fillFdf(ctx, out, *fillInp, *fillKeyvals...),
			fmt.Sprintf("fillPdf out=%q inp=%q keyvals=%q", out, *fillInp, *fillKeyvals))
	}
}

func splitPdfZip(ctx context.Context, outfn, inpfn string) error {
	var changed bool
	if inpfn, changed = ensureFilename(inpfn, false); changed {
		defer func() { _ = os.Remove(inpfn) }()
	}
	filenames, err := converter.PdfSplit(ctx, inpfn)
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

func cleanPdf(ctx context.Context, outfn, inpfn string) error {
	var changed bool
	fmt.Fprintf(os.Stderr, "inpfn=%s outfn=%s\n", inpfn, outfn)
	if inpfn, changed = ensureFilename(inpfn, false); changed {
		defer func() { _ = os.Remove(inpfn) }()
	}
	outfn, changed = ensureFilename(outfn, true)
	fmt.Fprintf(os.Stderr, "inpfn=%s outfn=%s\n", inpfn, outfn)
	if err := converter.PdfRewrite(ctx, outfn, inpfn); err != nil {
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

func countPdf(ctx context.Context, inpfn string) error {
	n, err := converter.PdfPageNum(ctx, inpfn)
	if err != nil {
		return err
	}
	fmt.Printf("%d\n", n)
	return nil
}

func fillFdf(ctx context.Context, outfn, inpfn string, kv ...string) error {
	values := make(map[string]string, len(kv))
	for _, txt := range kv {
		i := strings.IndexByte(txt, '=')
		if i < 0 {
			logger.Log("no = in key=value arg!")
			continue
		}
		values[txt[:i]] = txt[i+1:]
	}
	return converter.PdfFillFdf(ctx, outfn, inpfn, values)
}
