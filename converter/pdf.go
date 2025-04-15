// Copyright 2017, 2023 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"golang.org/x/sync/errgroup"

	"github.com/google/renameio/v2"
	"github.com/tgulacsi/go/pdf"
)

var popplerOk = map[string]string{"pdfinfo": "", "pdfseparate": "", "pdfunite": ""}

const (
	pcNotChecked = 0
	pcNothing    = -1
	pcPdfClean   = 1
	pcMutool     = 2
)

// PdfPageNum returns the number of pages
func PdfPageNum(ctx context.Context, srcfn string) (numberofpages int, err error) {
	if err = ctx.Err(); err != nil {
		return -1, err
	}
	if numberofpages, _, err = pdfPageNum(ctx, srcfn); err == nil {
		return
	}
	if err = PdfClean(ctx, srcfn); err != nil {
		logger.Info("ERROR PdfClean", "file", srcfn, "error", err)
	}
	numberofpages, _, err = pdfPageNum(ctx, srcfn)
	return
}

func pdfPageNum(ctx context.Context, srcfn string) (numberofpages int, encrypted bool, err error) {
	numberofpages = -1
	if err = ctx.Err(); err != nil {
		return
	}

	if numberofpages, err = pdf.PageNum(ctx, srcfn); err == nil {
		return numberofpages, false, nil
	}

	pdfinfo := false
	var cmd *cmd
	if popplerOk["pdfinfo"] != "" {
		// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		cmd = Exec.CommandContext(ctx, popplerOk["pdfinfo"], srcfn)
		pdfinfo = true
	} else {
		// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		cmd = Exec.CommandContext(ctx, *ConfPdftk, srcfn, "dump_data_utf8")
	}
	out, e := cmd.CombinedOutput()
	err = e
	if len(out) == 0 {
		return
	}

	getLine := func(hay []byte, needle string) (ret string) {
		i := bytes.Index(hay, []byte("\n"+needle))
		if i >= 0 {
			line := hay[i+1+len(needle):]
			j := bytes.IndexByte(line, '\n')
			if j >= 0 {
				return string(bytes.Trim(line[:j], " \t\r\n"))
			}
		}
		return ""
	}

	if pdfinfo {
		encrypted = getLine(out, "Encrypted:") == "yes"
		s := getLine(out, "Pages:")
		if s == "" {
			err = ErrBadPDF
		} else {
			numberofpages, err = strconv.Atoi(s)
		}
	} else {
		encrypted = bytes.Contains(out, []byte(" password "))
		s := getLine(out, "NumberOfPages:")
		if s == "" {
			err = ErrBadPDF
		} else {
			numberofpages, err = strconv.Atoi(s)
		}
	}
	return
}

var ErrBadPDF = errors.New("bad pdf")

// PdfSplit splits pdf to pages, returns those filenames
func PdfSplit(ctx context.Context, srcfn string, pages []uint16) (filenames []string, cleanup func() error, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cleanup = func() error { return nil }
	if err = ctx.Err(); err != nil {
		return
	}
	pageNum, e := PdfPageNum(ctx, srcfn)
	if e != nil {
		err = fmt.Errorf("cannot determine page number of %s: %w", srcfn, e)
		return
	} else if pageNum == 0 {
		logger.Info("0 pages", "file", srcfn)
	} else if pageNum == 1 {
		filenames = append(filenames, srcfn)
		return
	}

	if !filepath.IsAbs(srcfn) {
		if srcfn, err = filepath.Abs(srcfn); err != nil {
			return
		}
	}
	destdir, dErr := os.MkdirTemp(Workdir, filepath.Base(srcfn)+"-*-split")
	if dErr != nil {
		err = dErr
		return
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(destdir)
		} else {
			cleanup = func() error { return os.RemoveAll(destdir) }
		}
	}()

	prefix := strings.TrimSuffix(filepath.Base(srcfn), ".pdf") + "_"
	prefix = strings.Replace(prefix, "%", "!P!", -1)

	srcFi, err := os.Stat(srcfn)
	if err != nil {
		return filenames, cleanup, err
	}

	if *ConfMutool != "" {
		logger.Info(*ConfMutool, "src", srcfn, "dest", destdir)
		grp, grpCtx := errgroup.WithContext(ctx)
		grp.SetLimit(Concurrency)
		pp := pages
		if len(pp) == 0 {
			pp = make([]uint16, 0, pageNum)
			for i := 0; i < pageNum; i++ {
				pp = append(pp, uint16(i))
			}
		}
		for _, p := range pp {
			p := int(p)
			grp.Go(func() error {
				if err = callAt(grpCtx, *ConfMutool, filepath.Dir(srcfn), "draw",
					"-o", filepath.Join(destdir, fmt.Sprintf(prefix+"%03d.pdf", p)),
					"-L", "-N", "-q", "-F", "pdf",
					srcfn,
					strconv.Itoa(p),
				); err != nil {
					return fmt.Errorf("executing %q: %w", *ConfMutool, err)
				}
				return nil
			})
		}
		if err = grp.Wait(); err != nil {
			return
		}
	} else if pdfsep := popplerOk["pdfseparate"]; pdfsep != "" {
		logger.Info(pdfsep, "src", srcfn, "dest", destdir)
		restArgs := []string{srcfn, filepath.Join(destdir, prefix+"%03d.pdf")}
		if len(pages) != 0 && (len(pages) == 1 || len(pages) <= pageNum/2) {
			args := append(append(make([]string, 0, 4+len(restArgs)),
				"-f", "", "-l", ""), restArgs...)
			for _, p := range pages {
				ps := strconv.FormatUint(uint64(p), 10)
				args[1], args[3] = ps, ps
				logger.Info("pdfsep", "at", destdir, "args", args)
				if err = callAt(ctx, pdfsep, destdir, args...); err != nil {
					err = fmt.Errorf("executing %s: %w", pdfsep, err)
					return
				}
			}
		} else {
			logger.Info("pdfsep", "at", destdir, "args", restArgs)
			if err = callAt(ctx, pdfsep, destdir, restArgs...); err != nil {
				err = fmt.Errorf("executing %s: %w", pdfsep, err)
				return
			}
		}
	} else {
		logger.Info(*ConfPdftk, "src", srcfn, "dest", destdir)
		if err = callAt(ctx, *ConfPdftk, destdir, srcfn, "burst", "output", prefix+"%03d.pdf"); err != nil {
			err = fmt.Errorf("executing %s: %w", *ConfPdftk, err)
			return
		}
	}
	dh, e := os.Open(destdir)
	if e != nil {
		err = fmt.Errorf("opening destdir %s: %w", destdir, e)
		return
	}
	defer func() { _ = dh.Close() }()
	if filenames, err = dh.Readdirnames(-1); err != nil {
		err = fmt.Errorf("listing %s: %w", dh.Name(), err)
		return
	}
	logger.Info("ls", "destDir", destdir, "files", filenames)
	names := make([]string, 0, len(filenames))
	format := "%d"
	if n := len(filenames); n > 9999 {
		format = "%05d"
	} else if n > 999 {
		format = "%04d"
	} else if n > 99 {
		format = "%03d"
	} else if n > 9 {
		format = "%02d"
	}
	for _, fn := range filenames {
		if !strings.HasSuffix(fn, ".pdf") {
			continue
		}
		if !strings.HasPrefix(fn, prefix) {
			logger.Info("mismatch", "fn", fn, "prefix", prefix)
			continue
		}
		var i int
		for _, r := range fn[len(prefix):] {
			if '0' <= r && r <= '9' {
				i++
			} else {
				break
			}
		}
		//logger.Info("", "prefix", prefix, "fn", fn, "i", i)
		n, iErr := strconv.ParseUint(fn[len(prefix):len(prefix)+i], 10, 16)
		if iErr != nil {
			err = fmt.Errorf("%q: %w", fn, iErr)
			return
		}
		if len(pages) != 0 {
			u := uint16(n)
			var found bool
			for _, p := range pages {
				if found = p == u; found {
					break
				}
			}
			if !found {
				logger.Info("skip", "page", u, "file", fn)
				_ = os.Remove(filepath.Join(destdir, fn))
				continue
			}
		}

		if *ConfGm != "" {
			nFi, err := os.Stat(filepath.Join(destdir, fn))
			if err != nil {
				logger.Info("stat", "fn", fn, "error", err)
				continue
			}
			logger.Info("may-resample", "file", fn, "size", nFi.Size(),
				"src", srcFi.Name(), "srcSize", srcFi.Size())
			if nFi.Size() >= srcFi.Size()*9/10 {
				gFn := fn + ".gm.pdf"
				if err = callAt(ctx, *ConfGm, destdir,
					"convert", fn, "-resample", "300x300", gFn,
				); err != nil {
					logger.Info("gm convert", "fn", fn, "error", err)
				} else if gFi, err := os.Stat(filepath.Join(destdir, gFn)); err != nil {
					logger.Info("stat", "gFn", gFn, "error", err)
				} else if gFi.Size() >= nFi.Size()/2 {
					logger.Info("not smaller", "fn", fn, "oSize", nFi.Size(), "nSize", gFi.Size())
				} else {
					logger.Info("replace split pdf with gm convert'd", "fn", fn, "oSize", nFi.Size(), "nSize", gFi.Size())
					_ = os.Rename(filepath.Join(destdir, gFn), filepath.Join(destdir, fn))
				}
			}
		}

		nfn := fn[:len(prefix)] + fmt.Sprintf(format, n) + ".pdf"
		if nfn != fn {
			if err = os.Rename(filepath.Join(dh.Name(), fn), filepath.Join(dh.Name(), nfn)); err != nil {
				return
			}
		}
		names = append(names, nfn)
	}
	filenames = names
	sort.Strings(filenames)
	logger.Info("splitted", "names", filenames)
	for i, fn := range filenames {
		filenames[i] = filepath.Join(destdir, fn)
	}
	return filenames, cleanup, nil
}

// PdfMerge merges pdf files into destfn
func PdfMerge(ctx context.Context, destfn string, filenames ...string) error {
	logger.Debug("PdfMerge", "filenames", len(filenames))
	if len(filenames) == 0 {
		return errors.New("filenames required")
	} else if len(filenames) == 1 {
		if err := os.Link(filenames[0], destfn); err == nil {
			return nil
		}
		sfh, err := os.Open(filenames[0])
		if err != nil {
			return err
		}
		defer sfh.Close()
		dfh, err := renameio.NewPendingFile(destfn)
		if err != nil {
			return err
		}
		defer dfh.Close()
		if _, err = io.Copy(dfh, sfh); err != nil {
			return err
		}
		return dfh.CloseAtomicallyReplace()
	}
	err := pdfMerge(ctx, destfn, filenames...)
	if err == nil {
		return nil
	}
	logger.Error("pdfMerge", "error", err)

	// filter out bad PDFs
	fns := make([]string, 0, len(filenames))
	for _, fn := range filenames {
		if err := ctx.Err(); err != nil {
			return err
		}
		if n, err := PdfPageNum(ctx, fn); n > 0 && err == nil {
			fns = append(fns, fn)
		} else {
			logger.Info("merge SKIP", "file", fn, "pages", n, "error", err)
		}
	}

	return pdfMerge(ctx, destfn, fns...)
}

func pdfMerge(ctx context.Context, destfn string, filenames ...string) error {
	if err := pdf.MergeFiles(ctx, destfn, filenames...); err == nil {
		if fi, err := os.Stat(destfn); err == nil {
			logger.Debug("pdf.MergeFiles", "dest", fi.Size())
			if fi.Size() > 5 {
				return nil
			}
		}
	}

	if gotenberg.Valid() {
		err := gotenberg.PostFileNames(ctx, destfn, "/forms/pdfengines/merge", filenames, "application/pdf")
		logger.Debug("gotenberg.MergePDF", "error", err)
		if err == nil {
			return nil
		}
	}

	var buf bytes.Buffer
	pdfunite := popplerOk["pdfunite"]
	if pdfunite != "" {
		args := append(append(make([]string, 0, len(filenames)+1), filenames...),
			destfn)
		// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		cmd := Exec.CommandContext(ctx, pdfunite, args...)
		cmd.Stdout = io.MultiWriter(&buf, os.Stdout)
		cmd.Stderr = io.MultiWriter(&buf, os.Stderr)
		err := cmd.Run()
		logger.Debug("pdfunite", "cmd", cmd.Args, "error", err)
		if err == nil {
			return nil
		}
		err = fmt.Errorf("%q: %w", cmd.Args, err)
		logger.Info("WARN pdfunite failed", "error", err, "errTxt", buf.String())
	}
	args := append(append(make([]string, 0, len(filenames)+3), filenames...),
		"cat", "output", destfn)
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := Exec.CommandContext(ctx, *ConfPdftk, args...)
	cmd.Stdout = io.MultiWriter(&buf, os.Stdout)
	cmd.Stderr = io.MultiWriter(&buf, os.Stderr)
	err := cmd.Run()
	logger.Debug("pdftk", "cmd", cmd.Args, "error", err)
	if err != nil {
		err = fmt.Errorf("%q: %w", cmd.Args, err)
		return fmt.Errorf("%s: %w", buf.String(), err)
	}
	return nil
}

var (
	alreadyCleaned = make(map[string]bool, 16)
	cleanMtx       = sync.Mutex{}
	pdfCleanStatus = int(0)
)

func getHash(fn string) string {
	fh, err := os.Open(fn)
	if err != nil {
		logger.Info("WARN getHash open", "fn", fn, "error", err)
		return ""
	}
	hsh := sha256.New()
	_, err = io.Copy(hsh, fh)
	_ = fh.Close()
	if err != nil {
		logger.Info("WARN getHash reading", "fn", fn, "error", err)
	}
	return base64.URLEncoding.EncodeToString(hsh.Sum(nil))
}

func isAlreadyCleaned(fn string) bool {
	if !filepath.IsAbs(fn) {
		afn, err := filepath.Abs(fn)
		if err != nil {
			logger.Info("WARN cannot absolutize filename", "fn", fn, "error", err)
		}
		fn = afn
	}
	cleanMtx.Lock()
	defer cleanMtx.Unlock()
	if _, ok := alreadyCleaned[fn]; ok {
		return true
	}
	hsh := getHash(fn)
	if hsh == "" {
		return false
	}
	if _, ok := alreadyCleaned[hsh]; ok {
		return true
	}
	return false
}

// PdfClean cleans PDF from restrictions
func PdfClean(ctx context.Context, fn string) (err error) {
	if !filepath.IsAbs(fn) {
		if fn, err = filepath.Abs(fn); err != nil {
			return
		}
	}
	if ok := isAlreadyCleaned(fn); ok {
		logger.Info("PdfClean already cleaned.", "file", fn)
		return nil
	}
	cleanMtx.Lock()
	if pdfCleanStatus == pcNotChecked { //first check
		pdfCleanStatus = pcNothing
		if ConfPdfClean != nil && *ConfPdfClean != "" {
			if _, e := exec.LookPath(*ConfPdfClean); e != nil {
				logger.Info("no pdfclean exists?", "pdfclean", *ConfPdfClean, "error", e)
			} else {
				pdfCleanStatus = pcPdfClean
			}
		}
		if ConfMutool != nil && *ConfMutool != "" {
			if _, e := exec.LookPath(*ConfMutool); e != nil {
				logger.Info("no mutool exists?", "mutool", *ConfMutool, "error", e)
			} else {
				if pdfCleanStatus == pcNothing {
					pdfCleanStatus = pcMutool
				} else {
					pdfCleanStatus |= pcMutool
				}
			}
		}
	}
	pdfCleanStatus := pdfCleanStatus // to be able to unlock
	cleanMtx.Unlock()

	var cleaned, encrypted bool
	if pdfCleanStatus != pcNothing {
		var cleaner string
		if pdfCleanStatus&pcPdfClean != 0 {
			cleaner = *ConfPdfClean
			err = call(ctx, cleaner, "-ggg", fn, fn+"-cleaned.pdf")
		} else {
			cleaner = *ConfMutool
			err = call(ctx, cleaner, "clean", "-ggg", fn, fn+"-cleaned.pdf")
		}
		if err != nil {
			if errS := err.Error(); strings.Contains(errS, " password ") || strings.Contains(errS, " password:") {
				err = fmt.Errorf("%v: %w", err, ErrPasswordProtected)
			}
			return fmt.Errorf("clean with %s: %w", cleaner, err)
		}
		cleaned = true
		_, encrypted, _ = pdfPageNum(ctx, fn+"-cleaned.pdf")
		if encrypted {
			logger.Info("WARN "+cleaner+": encrypted!", "file", fn)
		}
	}
	if !cleaned || encrypted {
		if err = PdfRewrite(ctx, fn+"-cleaned.pdf", fn); err != nil {
			return
		}
	}
	if err = os.Rename(fn+"-cleaned.pdf", fn); err != nil {
		return
	}
	cleanMtx.Lock()
	if len(alreadyCleaned) > 1024 {
		alreadyCleaned = make(map[string]bool, 16)
	}
	alreadyCleaned[fn] = true
	if hsh := getHash(fn); hsh != "" {
		alreadyCleaned[hsh] = true
	}
	cleanMtx.Unlock()
	return nil
}

func call(ctx context.Context, what string, args ...string) error {
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := Exec.CommandContext(ctx, what, args...)
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	return execute(cmd)
}

func callAt(ctx context.Context, what, where string, args ...string) error {
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := Exec.CommandContext(ctx, what, args...)
	cmd.Stderr = os.Stderr
	cmd.Dir = where
	return execute(cmd)
}

func execute(cmd *cmd) error {
	errout := bytes.NewBuffer(nil)
	cmd.Stderr = errout
	cmd.Stdout = cmd.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%#v while converting %s: %w", cmd, errout.Bytes(), err)
	}
	if len(errout.Bytes()) > 0 {
		logger.Info("WARN executes", "cmd", cmd, "error", errout.String())
	}
	return nil
}

var ErrPasswordProtected = errors.New("password protected")

func xToX(ctx context.Context, destfn, srcfn string, tops bool) (err error) {
	var gsOpts []string
	if tops {
		gsOpts = []string{"-q", "-dNOPAUSE", "-dBATCH", "-P-", "-dSAFER",
			"-sDEVICE=ps2write", "-sOutputFile=" + destfn, "-c", "save", "pop",
			"-f", srcfn}
	} else {
		gsOpts = []string{"-P-", "-dSAFER", "-dNOPAUSE", "-dCompatibilityLevel=1.4",
			"-dPDFSETTINGS=/printer", "-dUseCIEColor=true",
			"-q", "-dBATCH", "-sDEVICE=pdfwrite", "-sstdout=%stderr",
			"-sOutputFile=" + destfn,
			"-P-", "-dSAFER", "-dCompatibilityLevel=1.4",
			"-c", ".setpdfwrite", "-f", srcfn}
	}

	if err = call(ctx, *ConfGs, gsOpts...); err != nil {
		if strings.Contains(err.Error(), " password ") {
			err = fmt.Errorf("%+v: %w", err, ErrPasswordProtected)
		}
		return fmt.Errorf("converting %s to %s with %s: %w",
			srcfn, destfn, *ConfGs, err)
	}
	return nil
}

// PdfToPs converts PDF to postscript
func PdfToPs(ctx context.Context, destfn, srcfn string) error {
	return xToX(ctx, destfn, srcfn, true)
}

// PsToPdf converts postscript to PDF
func PsToPdf(ctx context.Context, destfn, srcfn string) error {
	return xToX(ctx, destfn, srcfn, false)
}

// PdfRewrite converts PDF to PDF (rewrites as PDF->PS->PDF)
func PdfRewrite(ctx context.Context, destfn, srcfn string) error {
	var err error
	psfn := nakeFilename(srcfn) + "-pp.ps"
	if err = PdfToPs(ctx, psfn, srcfn); err != nil {
		return err
	}
	if !LeaveTempFiles {
		defer func() { _ = unlink(psfn, "rewritten") }()
	}
	var pdffn2 string
	if destfn == srcfn {
		pdffn2 = psfn + ".pdf"
	} else {
		pdffn2 = destfn
	}
	if err = PsToPdf(ctx, pdffn2, psfn); err != nil {
		return err
	}
	return moveFile(pdffn2, destfn)
}

// PdfDumpFields dumps the field names from the given PDF.
func PdfDumpFields(ctx context.Context, inpfn string) ([]string, error) {
	var buf bytes.Buffer
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := Exec.CommandContext(ctx, *ConfPdftk, inpfn, "dump_data_fields_utf8", "output", "-")
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	var fields []string
	for _, line := range bytes.Split(buf.Bytes(), []byte("\n")) {
		if bytes.HasPrefix(line, []byte("FieldName: ")) {
			fields = append(fields, string(bytes.TrimSpace(line[11:])))
		}
	}
	return fields, nil
}

// PdfDumpFdf dumps the FDF from the given PDF.
func PdfDumpFdf(ctx context.Context, destfn, inpfn string) error {
	if err := call(ctx, *ConfPdftk, inpfn, "generate_fdf", "output", destfn); err != nil {
		return fmt.Errorf("pdftk generate_fdf: %w", err)
	}
	return nil
}

var fillFdfMu sync.Mutex

// PdfFillFdf fills the FDF and generates PDF.
func PdfFillFdf(ctx context.Context, destfn, inpfn string, values map[string]string) error {
	if len(values) == 0 {
		return copyFile(inpfn, destfn)
	}
	fp, err := getFdf(ctx, inpfn)
	if err != nil {
		return err
	}
	for k, v := range values {
		if err = fp.Set(k, v); err != nil {
			return err
		}
	}
	var buf bytes.Buffer
	if _, err = fp.WriteTo(&buf); err != nil {
		return err
	}

	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := Exec.CommandContext(ctx, *ConfPdftk, inpfn, "fill_form", "-", "output", destfn)
	cmd.Stdin = bytes.NewReader(buf.Bytes())
	return execute(cmd)
}

func getFdf(ctx context.Context, inpfn string) (fieldParts, error) {
	var fp fieldParts
	hsh, err := fileContentHash(inpfn)
	if err != nil {
		return fp, err
	}
	fdfFn := filepath.Join(Workdir, base64.URLEncoding.EncodeToString(hsh.Sum(nil))+".fdf")
	if f, gobErr := os.Open(fdfFn + ".gob"); gobErr == nil {
		err = gob.NewDecoder(f).Decode(&fp)
		f.Close()
		if err == nil {
			return fp, nil
		}
		logger.Info("decoding", "file", f.Name(), "error", err)
	}

	fdf, fdfErr := os.ReadFile(fdfFn)
	if fdfErr != nil {
		_ = os.Remove(fdfFn)

		fillFdfMu.Lock()
		err = PdfDumpFdf(ctx, fdfFn, inpfn)
		fillFdfMu.Unlock()
		if err != nil {
			return fp, err
		}
		if fdf, err = os.ReadFile(fdfFn); err != nil {
			return fp, err
		}
	}

	fp = splitFdf(fdf)
	//logger.Info("fdf", "len", len(fdf), "split", fp)

	f, err := os.OpenFile(fdfFn+".gob", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		logger.Info("cannot create", "file", fdfFn+".gob", "error", err)
	} else {
		fillFdfMu.Lock()
		err = gob.NewEncoder(f).Encode(fp)
		fillFdfMu.Unlock()
		if err != nil {
			logger.Info("encode gobv", "file", f.Name(), "error", err)
		} else {
			if err = f.Close(); err != nil {
				logger.Info("close", "file", f.Name(), "error", err)
				_ = os.Remove(f.Name())
			}
		}
	}

	return fp, nil
}

type FieldSetter interface {
	Set(key, value string) error
}

var fieldPartV = []byte("\n<<\n/V ()\n")

type fieldParts struct {
	Values map[string]string
	Parts  [][]byte
	Fields []string
}

func (fp fieldParts) WriteTo(w io.Writer) (n int64, err error) {
	length := len(fieldPartV)
	if length == 0 {
		return 0, io.EOF
	}
	fpv1, fpv2 := fieldPartV[:length-2], fieldPartV[length-2:]
	cew := &countErrWriter{w: w}
	length = len(fp.Parts) - 1
	if length < 0 {
		return 0, io.EOF
	}
	for i, part := range fp.Parts {
		if i == length {
			break
		}
		_, _ = cew.Write(part)
		var val string
		if i < len(fp.Fields) {
			val = fp.Values[fp.Fields[i]]
		}
		if len(val) == 0 {
			_, _ = cew.Write(fieldPartV)
		} else {
			_, _ = cew.Write(fpv1)
			_, _ = cew.Write([]byte{0xfe, 0xff})
			for _, u := range utf16.Encode([]rune(val)) {
				// http://stackoverflow.com/questions/6047970/weird-characters-when-filling-pdf-with-pdftk/19170162#19170162
				// UTF16-BE
				_, _ = cew.Write([]byte{byte(u >> 8), byte(u & 0xff)})
			}
			_, _ = cew.Write(fpv2)
		}
		if cew.err != nil {
			break
		}
	}
	_, _ = cew.Write(fp.Parts[length])
	return cew.n, cew.err
}

func (fp fieldParts) Set(key, value string) error {
	if _, ok := fp.Values[key]; !ok {
		logger.Info("unknown field", "field", fp.Fields)
		return fmt.Errorf("field %s not exist", key)
	}
	fp.Values[key] = value
	return nil
}

func splitFdf(fdf []byte) fieldParts {
	var fp fieldParts
	shortFieldPartV := []byte("\n/V ()\n")
	for {
		i := bytes.Index(fdf, shortFieldPartV)
		if i < 0 {
			fp.Parts = append(fp.Parts, fdf)
			break
		}
		fp.Parts = append(fp.Parts, bytes.TrimSuffix(fdf[:i], []byte("\n<<")))
		fdf = fdf[i+len(shortFieldPartV):]
	}
	fp.Fields = make([]string, 0, len(fp.Parts)-1)
	fp.Values = make(map[string]string, len(fp.Parts)-1)
	for _, part := range fp.Parts {
		i := bytes.Index(part, []byte("/T ("))
		if i < 0 {
			continue
		}
		j := bytes.Index(part[i:], []byte(")\n"))
		if j < 0 {
			continue
		}
		j += i
		key := string(part[i+4 : j])
		fp.Fields = append(fp.Fields, key)
		fp.Values[key] = ""
	}
	return fp
}

var _ io.Writer = (*countErrWriter)(nil)

type countErrWriter struct {
	w   io.Writer
	err error
	n   int64
}

func (cew *countErrWriter) Write(p []byte) (int, error) {
	if cew.err != nil {
		return 0, cew.err
	}
	n, err := cew.w.Write(p)
	cew.n += int64(n)
	cew.err = err
	return n, err
}
