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
	"math/rand"
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
func PdfSplit(ctx context.Context, srcfn string) (filenames []string, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if n, e := PdfPageNum(ctx, srcfn); err != nil {
		err = fmt.Errorf("cannot determine page number of %s: %w", srcfn, e)
		return
	} else if n == 0 {
		err = errors.New("0 pages in " + srcfn)
		return
	} else if n == 1 {
		filenames = append(filenames, srcfn)
		return
	}

	if !filepath.IsAbs(srcfn) {
		if srcfn, err = filepath.Abs(srcfn); err != nil {
			return
		}
	}
	destdir := filepath.Join(Workdir,
		filepath.Base(srcfn)+"-"+strconv.Itoa(rand.Int())+"-split") //nolint:gas
	if !fileExists(destdir) {
		if err = os.Mkdir(destdir, 0755); err != nil {
			return
		}
	}
	prefix := strings.TrimSuffix(filepath.Base(srcfn), ".pdf") + "_"
	prefix = strings.Replace(prefix, "%", "!P!", -1)

	srcFi, err := os.Stat(srcfn)
	if err != nil {
		return filenames, err
	}

	if pdfsep := popplerOk["pdfseparate"]; pdfsep != "" {
		Log("msg", pdfsep, "src", srcfn, "dest", destdir)
		if err = callAt(ctx, pdfsep,
			destdir,
			srcfn,
			filepath.Join(destdir, prefix+"%03d.pdf"),
		); err != nil {
			err = fmt.Errorf("executing %s: %w", pdfsep, err)
			return
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

		if *ConfGm != "" {
			nFi, err := os.Stat(filepath.Join(destdir, fn))
			if err != nil {
				Log("msg", "stat", "fn", fn, "error", err)
				continue
			}
			if nFi.Size() > srcFi.Size()/3 {
				gFn := fn + ".gm.pdf"
				if err = callAt(ctx, *ConfGm, destdir,
					"convert", fn, "-density", "300x300", gFn,
				); err != nil {
					Log("msg", "gm convert", "fn", fn, "error", err)
				} else if gFi, err := os.Stat(filepath.Join(destdir, gFn)); err != nil {
					Log("msg", "stat", "gFn", gFn, "error", err)
				} else if gFi.Size() >= nFi.Size()/3 {
					Log("msg", "not smaller", "fn", fn, "oSize", nFi.Size(), "nSize", gFi.Size())
				} else {
					Log("msg", "replace split pdf with gm convert'd", "fn", fn, "oSize", nFi.Size(), "nSize", gFi.Size())
					os.Rename(filepath.Join(destdir, gFn), filepath.Join(destdir, fn))
				}
			}
		}

		n, iErr := strconv.Atoi(fn[len(prefix) : len(fn)-4])
		if iErr != nil {
			err = fmt.Errorf("%q: %w", fn, iErr)
			return
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
	return filenames, nil
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
		pw.CloseWithError(err)
		return fields, fmt.Errorf("pdftk generate_fdf: %w", err)
	}
	pw.Close()
	wg.Wait()
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
	fdfFn := filepath.Join(Workdir, base64.URLEncoding.EncodeToString(hsh.Sum(nil))+".fdf")
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
		if _, ok := fdfErr.(*os.PathError); !ok {
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
		cew.Write(part)
		val := fp.Values[fp.Fields[i]]
		if len(val) == 0 {
			cew.Write(fieldPartV)
		} else {
			cew.Write(fpv1)
			cew.Write([]byte{0xfe, 0xff})
			for _, u := range utf16.Encode([]rune(val)) {
				// http://stackoverflow.com/questions/6047970/weird-characters-when-filling-pdf-with-pdftk/19170162#19170162
				// UTF16-BE
				cew.Write([]byte{byte(u >> 8), byte(u & 0xff)})
			}
			cew.Write(fpv2)
		}
		if cew.err != nil {
			break
		}
	}
	cew.Write(fp.Parts[length])
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
