// Copyright 2017, 2026 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/UNO-SOFT/cli"
	"github.com/tgulacsi/agostle/converter"
)

func init() {
	pdfCmd := &cli.Command{Name: "pdf", Summary: "pdf commands"}
	subcommands = append(subcommands, pdfCmd)

	var out string
	withOutFlag := func(name string) *flag.FlagSet {
		fs := flag.NewFlagSet(name, flag.ContinueOnError)
		fs.StringVar(&out, "out", "", "output file")
		// TODO: "-o"
		return fs
	}
	{
		var sort bool
		fs := withOutFlag("merge")
		fs.BoolVar(&sort, "sort", false, "shall we sort the files by name before merge?")
		mergeCmd := cli.Command{Name: "merge", Flags: fs,
			Summary: "merges the given PDFs into one",
			Exec: func(ctx context.Context, state *cli.State) error {
				args := state.Args
				for i, s := range args {
					if s == "" {
						args[i] = "-"
					}
				}
				if err := mergePdf(ctx, out, args, sort); err != nil {
					return fmt.Errorf("mergePDF out=%q sort=%v inp=%v: %w", out, sort, args, err)
				}
				return nil
			},
		}
		pdfCmd.SubCommands = append(pdfCmd.SubCommands, &mergeCmd)
	}

	fs := withOutFlag("split")
	flagSplitPages := fs.String("pages", "", "pages (comma separated)")
	splitCmd := cli.Command{Name: "split", Flags: fs,
		Summary: "splits the given PDF into one per page",
		Exec: func(ctx context.Context, state *cli.State) error {
			var splitInp string
			args := state.Args
			if len(args) != 0 {
				splitInp = args[0]
			}
			if splitInp == "" {
				splitInp = "-"
			}
			if err := splitPdfZip(ctx, out, splitInp, parseUint16s(strings.Split(*flagSplitPages, ","))); err != nil {
				return fmt.Errorf("splitPdfZip out=%q inp=%q: %w", out, splitInp, err)
			}
			return nil
		},
	}
	pdfCmd.SubCommands = append(pdfCmd.SubCommands, &splitCmd)

	countCmd := cli.Command{Name: "count",
		Summary: "prints the number of pages in the given pdf",
		Exec: func(ctx context.Context, state *cli.State) error {
			var countInp string
			args := state.Args
			if len(args) != 0 {
				countInp = args[0]
			}
			if err := countPdf(ctx, countInp); err != nil {
				return fmt.Errorf("countPdf inp=%s: %w", countInp, err)
			}
			return nil
		},
	}
	pdfCmd.SubCommands = append(pdfCmd.SubCommands, &countCmd)

	fs = withOutFlag("clean")
	cleanCmd := cli.Command{Name: "clean", Flags: fs,
		Summary: "clean PDF from encryption",
		Exec: func(ctx context.Context, state *cli.State) error {
			var cleanInp string
			args := state.Args
			if len(args) != 0 {
				cleanInp = args[0]
			}
			if err := cleanPdf(ctx, out, cleanInp); err != nil {
				return fmt.Errorf("cleanPdf out=%q inp=%q: %w", out, cleanInp, err)
			}
			return nil
		},
	}
	pdfCmd.SubCommands = append(pdfCmd.SubCommands, &cleanCmd)

	{
		var mime string
		fs = withOutFlag("topdf")
		fs.StringVar(&mime, "mime", "application/octet-stream", "input mimetype")
		topdfCmd := cli.Command{Name: "topdf", Flags: fs,
			Summary: "tries to convert the given file (you can specify its mime-type) to PDF",
			Exec: func(ctx context.Context, state *cli.State) error {
				var topdfInp string
				args := state.Args
				if len(args) != 0 {
					topdfInp = args[0]
				}
				if err := toPdf(out, topdfInp, mime); err != nil {
					return fmt.Errorf("topdf out=%q inp=%q mime=%q: %w", out, topdfInp, mime, err)
				}
				return nil
			},
		}
		pdfCmd.SubCommands = append(pdfCmd.SubCommands, &topdfCmd)
	}

	fs = withOutFlag("fill")
	fillPdfCmd := cli.Command{Name: "fill", Flags: fs,
		Summary: "fill PDF form",
		Usage: `fill PDF form
input.pdf key1=value1 key2=value2...`,
		Exec: func(ctx context.Context, state *cli.State) error {
			var fillInp string
			var fillKeyvals []string
			args := state.Args
			if len(args) != 0 {
				fillInp = args[0]
				fillKeyvals = args[1:]
			}
			if err := fillFdf(ctx, out, fillInp, fillKeyvals...); err != nil {
				return fmt.Errorf("fillPdf out=%q inp=%q keyvals=%q: %w", out, fillInp, fillKeyvals, err)
			}
			return nil
		},
	}
	pdfCmd.SubCommands = append(pdfCmd.SubCommands, &fillPdfCmd)
}

func splitPdfZip(ctx context.Context, outfn, inpfn string, pages []uint16) error {
	var changed bool
	if inpfn, changed = ensureFilename(inpfn, false); changed {
		defer func() { _ = os.Remove(inpfn) }()
	}
	filenames, cleanup, err := converter.PdfSplit(ctx, inpfn, pages)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup() }()
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

func mergePdf(ctx context.Context, outfn string, inpfn []string, sortFiles bool) error {
	if sortFiles {
		sort.Strings(inpfn)
	}
	return converter.PdfMerge(ctx, outfn, inpfn...)
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
			logger.Info("no = in key=value arg!")
			continue
		}
		values[txt[:i]] = txt[i+1:]
	}
	return converter.PdfFillFdf(ctx, outfn, inpfn, values)
}
