// Copyright 2017, 2020 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"

	"context"

	"github.com/tgulacsi/go/pdf"
	"github.com/tgulacsi/go/temp"
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
		Log("msg", "ERROR PdfClean", "file", srcfn, "error", err)
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
	var cmd *exec.Cmd
	if popplerOk["pdfinfo"] != "" {
		cmd = exec.CommandContext(ctx, popplerOk["pdfinfo"], srcfn)
		pdfinfo = true
	} else {
		cmd = exec.CommandContext(ctx, *ConfPdftk, srcfn, "dump_data_utf8")
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
	cleanup = func() error { return nil }
	if err = ctx.Err(); err != nil {
		return
	}
	pageNum, e := PdfPageNum(ctx, srcfn)
	if e != nil {
		err = fmt.Errorf("cannot determine page number of %s: %w", srcfn, e)
		return
	} else if pageNum == 0 {
		Log("msg", "0 pages", "file", srcfn)
	} else if pageNum == 1 {
		filenames = append(filenames, srcfn)
		return
	}

	if !filepath.IsAbs(srcfn) {
		if srcfn, err = filepath.Abs(srcfn); err != nil {
			return
		}
	}
	WorkdirMu.RLock()
	destdir, dErr := os.MkdirTemp(Workdir, filepath.Base(srcfn)+"-*-split")
	WorkdirMu.RUnlock()
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

	if pdfsep := popplerOk["pdfseparate"]; pdfsep != "" {
		Log("msg", pdfsep, "src", srcfn, "dest", destdir)
		restArgs := []string{srcfn, filepath.Join(destdir, prefix+"%03d.pdf")}
		if len(pages) != 0 && (len(pages) == 1 || len(pages) <= pageNum/2) {
			args := append(append(make([]string, 0, 4+len(restArgs)),
				"-f", "", "-l", ""), restArgs...)
			for _, p := range pages {
				ps := strconv.FormatUint(uint64(p), 10)
				args[1], args[3] = ps, ps
				Log("msg", "pdfsep", "at", destdir, "args", args)
				if err = callAt(ctx, pdfsep, destdir, args...); err != nil {
					err = fmt.Errorf("executing %s: %w", pdfsep, err)
					return
				}
			}
		} else {
			Log("msg", "pdfsep", "at", destdir, "args", restArgs)
			if err = callAt(ctx, pdfsep, destdir, restArgs...); err != nil {
				err = fmt.Errorf("executing %s: %w", pdfsep, err)
				return
			}
		}
	} else {
		Log("msg", *ConfPdftk, "src", srcfn, "dest", destdir)
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
	Log("msg", "ls", "destDir", destdir, "files", filenames)
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
			Log("msg", "mismatch", "fn", fn, "prefix", prefix)
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
		//Log("msg", "", "prefix", prefix, "fn", fn, "i", i)
		n, iErr := strconv.Atoi(fn[len(prefix) : len(prefix)+i])
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
				Log("msg", "skip", "page", u, "file", fn)
				_ = os.Remove(filepath.Join(destdir, fn))
				continue
			}
		}

		if *ConfGm != "" {
			nFi, err := os.Stat(filepath.Join(destdir, fn))
			if err != nil {
				Log("msg", "stat", "fn", fn, "error", err)
				continue
			}
			Log("msg", "may-resample", "file", fn, "size", nFi.Size(),
				"src", srcFi.Name(), "srcSize", srcFi.Size())
			if nFi.Size() >= srcFi.Size()*9/10 {
				gFn := fn + ".gm.pdf"
				if err = callAt(ctx, *ConfGm, destdir,
					"convert", fn, "-resample", "300x300", gFn,
				); err != nil {
					Log("msg", "gm convert", "fn", fn, "error", err)
				} else if gFi, err := os.Stat(filepath.Join(destdir, gFn)); err != nil {
					Log("msg", "stat", "gFn", gFn, "error", err)
				} else if gFi.Size() >= nFi.Size()/2 {
					Log("msg", "not smaller", "fn", fn, "oSize", nFi.Size(), "nSize", gFi.Size())
				} else {
					Log("msg", "replace split pdf with gm convert'd", "fn", fn, "oSize", nFi.Size(), "nSize", gFi.Size())
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
	Log("msg", "splitted", "names", filenames)
	for i, fn := range filenames {
		filenames[i] = filepath.Join(destdir, fn)
	}
	return filenames, cleanup, nil
}

// PdfMerge merges pdf files into destfn
func PdfMerge(ctx context.Context, destfn string, filenames ...string) error {
	if len(filenames) == 0 {
		return errors.New("filenames required")
	} else if len(filenames) == 1 {
		os.Remove(destfn)
		return temp.LinkOrCopy(filenames[0], destfn)
	}
	if err := pdfMerge(ctx, destfn, filenames...); err == nil {
		return err
	}

	// filter out bad PDFs
	fns := make([]string, 0, len(filenames))
	for _, fn := range filenames {
		if err := ctx.Err(); err != nil {
			return err
		}
		if n, err := PdfPageNum(ctx, fn); n > 0 && err == nil {
			fns = append(fns, fn)
		} else {
			Log("msg", "merge SKIP", "file", fn, "pages", n, "error", err)
		}
	}

	return pdfMerge(ctx, destfn, fns...)
}

func pdfMerge(ctx context.Context, destfn string, filenames ...string) error {
	if err := pdf.MergeFiles(destfn, filenames...); err == nil {
		return nil
	}

	var buf bytes.Buffer
	pdfunite := popplerOk["pdfunite"]
	if pdfunite != "" {
		args := append(append(make([]string, 0, len(filenames)+1), filenames...),
			destfn)
		cmd := exec.CommandContext(ctx, pdfunite, args...)
		cmd.Stdout = io.MultiWriter(&buf, os.Stdout)
		cmd.Stderr = io.MultiWriter(&buf, os.Stderr)
		err := cmd.Run()
		if err == nil {
			return nil
		}
		err = fmt.Errorf("%q: %w", cmd.Args, err)
		Log("msg", "WARN pdfunite failed", "error", err, "errTxt", buf.String())
	}
	args := append(append(make([]string, 0, len(filenames)+3), filenames...),
		"cat", "output", destfn)
	cmd := exec.CommandContext(ctx, *ConfPdftk, args...)
	cmd.Stdout = io.MultiWriter(&buf, os.Stdout)
	cmd.Stderr = io.MultiWriter(&buf, os.Stderr)
	if err := cmd.Run(); err != nil {
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
		Log("msg", "WARN getHash open", "fn", fn, "error", err)
		return ""
	}
	hsh := sha1.New()
	_, err = io.Copy(hsh, fh)
	_ = fh.Close()
	if err != nil {
		Log("msg", "WARN getHash reading", "fn", fn, "error", err)
	}
	return base64.URLEncoding.EncodeToString(hsh.Sum(nil))
}

func isAlreadyCleaned(fn string) bool {
	var err error
	if !filepath.IsAbs(fn) {
		if fn, err = filepath.Abs(fn); err != nil {
			Log("msg", "WARN cannot absolutize filename", "fn", fn, "error", err)
		}
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
		Log("msg", "PdfClean already cleaned.", "file", fn)
		return nil
	}
	cleanMtx.Lock()
	if pdfCleanStatus == pcNotChecked { //first check
		pdfCleanStatus = pcNothing
		if ConfPdfClean != nil && *ConfPdfClean != "" {
			if _, e := exec.LookPath(*ConfPdfClean); e != nil {
				Log("msg", "no pdfclean exists?", "pdfclean", *ConfPdfClean, "error", e)
			} else {
				pdfCleanStatus = pcPdfClean
			}
		}
		if ConfMutool != nil && *ConfMutool != "" {
			if _, e := exec.LookPath(*ConfMutool); e != nil {
				Log("msg", "no mutool exists?", "mutool", *ConfMutool, "error", e)
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
			Log("msg", "WARN "+cleaner+": encrypted!", "file", fn)
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
	cmd := exec.CommandContext(ctx, what, args...)
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	return execute(cmd)
}

func callAt(ctx context.Context, what, where string, args ...string) error {
	cmd := exec.CommandContext(ctx, what, args...)
	cmd.Stderr = os.Stderr
	cmd.Dir = where
	return execute(cmd)
}

func execute(cmd *exec.Cmd) error {
	errout := bytes.NewBuffer(nil)
	cmd.Stderr = errout
	cmd.Stdout = cmd.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%#v while converting %s: %w", cmd, errout.Bytes(), err)
	}
	if len(errout.Bytes()) > 0 {
		Log("msg", "WARN executes", "cmd", cmd, "error", errout.String())
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
	pr, pw := io.Pipe()
	cmd := exec.CommandContext(ctx, *ConfPdftk, inpfn, "dump_data_fields_utf8", "output", "-")
	cmd.Stdout = pw
	var fields []string
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scan := bufio.NewScanner(pr)
		for scan.Scan() {
			if bytes.HasPrefix(scan.Bytes(), []byte("FieldName: ")) {
				fields = append(fields, string(bytes.TrimSpace(scan.Bytes()[11:])))
			}
		}
		if scan.Err() != nil {
			Log("msg", "scan", "error", scan.Err())
		}
	}()
	err := cmd.Run()
	if err != nil {
		_ = pw.CloseWithError(err)
		return fields, fmt.Errorf("pdftk generate_fdf: %w", err)
	}
	pw.Close()
	wg.Wait()
	return fields, err
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

	cmd := exec.CommandContext(ctx, *ConfPdftk, inpfn, "fill_form", "-", "output", destfn)
	cmd.Stdin = bytes.NewReader(buf.Bytes())
	return execute(cmd)
}

func getFdf(ctx context.Context, inpfn string) (fieldParts, error) {
	var fp fieldParts
	hsh, err := fileContentHash(inpfn)
	if err != nil {
		return fp, err
	}
	WorkdirMu.RLock()
	fdfFn := filepath.Join(Workdir, base64.URLEncoding.EncodeToString(hsh.Sum(nil))+".fdf")
	WorkdirMu.RUnlock()
	if f, gobErr := os.Open(fdfFn + ".gob"); gobErr == nil {
		err = gob.NewDecoder(f).Decode(&fp)
		f.Close()
		if err == nil {
			return fp, nil
		}
		Log("msg", "decoding", "file", f.Name(), "error", err)
	}

	fdf, fdfErr := ioutil.ReadFile(fdfFn)
	if fdfErr != nil {
		var pe *os.PathError
		if !errors.Is(fdfErr, pe) {
			Log("msg", "read fdf", "file", fdfFn, "error", fdfErr)
			os.Remove(fdfFn)
		} else {
			fillFdfMu.Lock()
			err = PdfDumpFdf(ctx, fdfFn, inpfn)
			fillFdfMu.Unlock()
			if err != nil {
				return fp, err
			}
			if fdf, err = ioutil.ReadFile(fdfFn); err != nil {
				return fp, err
			}
		}
	}

	fp = splitFdf(fdf)

	f, err := os.OpenFile(fdfFn+".gob", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		Log("msg", "cannot create", "file", fdfFn+".gob", "error", err)
	} else {
		fillFdfMu.Lock()
		err = gob.NewEncoder(f).Encode(fp)
		fillFdfMu.Unlock()
		if err != nil {
			Log("msg", "encode gobv", "file", f.Name(), "error", err)
		} else {
			if err = f.Close(); err != nil {
				Log("msg", "close", "file", f.Name(), "error", err)
				os.Remove(f.Name())
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
		val := fp.Values[fp.Fields[i]]
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
		Log("msg", "unknown field", "field", fp.Fields)
		return fmt.Errorf("field %s not exist", key)
	}
	fp.Values[key] = value
	return nil
}

func splitFdf(fdf []byte) fieldParts {
	var fp fieldParts
	for {
		i := bytes.Index(fdf, fieldPartV)
		if i < 0 {
			fp.Parts = append(fp.Parts, fdf)
			break
		}
		fp.Parts = append(fp.Parts, fdf[:i])
		fdf = fdf[i+len(fieldPartV):]
	}
	fp.Fields = make([]string, 0, len(fp.Parts)-1)
	fp.Values = make(map[string]string, len(fp.Parts)-1)
	for _, part := range fp.Parts {
		i := bytes.Index(part, []byte("/T ("))
		if i < 0 {
			continue
		}
		j := bytes.Index(part[i:], []byte(")\n>>"))
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
