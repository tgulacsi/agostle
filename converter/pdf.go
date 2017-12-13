// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/gob"
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

	"github.com/pkg/errors"

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
	if 0 == len(out) {
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
		numberofpages, err = strconv.Atoi(getLine(out, "Pages:"))
	} else {
		encrypted = bytes.Contains(out, []byte(" password "))
		numberofpages, err = strconv.Atoi(getLine(out, "NumberOfPages:"))
	}
	return
}

// PdfSplit splits pdf to pages, returns those filenames
func PdfSplit(ctx context.Context, srcfn string) (filenames []string, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if n, e := PdfPageNum(ctx, srcfn); err != nil {
		err = errors.Wrapf(e, "cannot determine page number of %s", srcfn)
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
		filepath.Base(srcfn)+"-"+strconv.Itoa(rand.Int())+"-split")
	if !fileExists(destdir) {
		if err = os.Mkdir(destdir, 0755); err != nil {
			return
		}
	}
	prefix := strings.Replace(filepath.Base(srcfn), "%", "!P!", -1) + "-"

	if popplerOk["pdfseparate"] != "" {
		if err = callAt(ctx, popplerOk["pdfseparate"],
			destdir,
			srcfn,
			filepath.Join(destdir, prefix+"%d.pdf"),
		); err != nil {
			err = errors.Wrapf(err, "executing %s", popplerOk["pdfseparate"])
			return
		}
	} else {
		if err = callAt(ctx, *ConfPdftk, destdir, srcfn, "burst", "output", prefix+"%03d.pdf"); err != nil {
			err = errors.Wrapf(err, "executing %s", *ConfPdftk)
			return
		}
	}
	dh, e := os.Open(destdir)
	if e != nil {
		err = errors.Wrapf(e, "opening destdir %s", destdir)
		return
	}
	defer func() { _ = dh.Close() }()
	if filenames, err = dh.Readdirnames(-1); err != nil {
		err = errors.Wrapf(err, "listing %s", dh.Name())
		return
	}
	//log.Printf("ls %s: %s", destdir, filenames)
	var (
		i  int
		fn string
	)
	for i = len(filenames) - 1; i >= 0; i-- {
		fn = filenames[i]
		//log.Printf("fn=%s prefix?%b suffix?%b", fn, strings.HasPrefix(fn, prefix),
		//strings.HasSuffix(fn, ".pdf"))
		if !(strings.HasPrefix(fn, prefix) && strings.HasSuffix(fn, ".pdf")) {
			if i >= len(filenames)-1 {
				filenames = filenames[:i]
			} else {
				filenames = append(filenames[:i], filenames[i+1:]...)
			}
		}
	}
	//log.Printf("splitted filenames: %s", filenames)
	sort.Strings(filenames)
	for i, fn = range filenames {
		filenames[i] = filepath.Join(destdir, fn)
	}
	return filenames, nil
}

// PdfMerge merges pdf files into destfn
func PdfMerge(ctx context.Context, destfn string, filenames ...string) error {
	if len(filenames) == 0 {
		return errors.New("filenames required!")
	} else if len(filenames) == 1 {
		return temp.LinkOrCopy(filenames[0], destfn)
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
		err = errors.Wrapf(err, "%q", cmd.Args)
		Log("msg", "WARN pdfunite failed", "error", err, "errTxt", buf.String())
	}
	args := append(append(make([]string, 0, len(filenames)+3), filenames...),
		"cat", "output", destfn)
	cmd := exec.CommandContext(ctx, *ConfPdftk, args...)
	cmd.Stdout = io.MultiWriter(&buf, os.Stdout)
	cmd.Stderr = io.MultiWriter(&buf, os.Stderr)
	if err := cmd.Run(); err != nil {
		err = errors.Wrapf(err, "%q", cmd.Args)
		return errors.Wrapf(err, buf.String())
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
	if _, err = io.Copy(hsh, fh); err != nil {
		Log("msg", "WARN getHash reading", "fn", fn, "error", err)
	}
	_ = fh.Close()
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
		Log("msg", "PdfClean file %q is already cleaned.", fn)
		return nil
	}
	cleanMtx.Lock()
	if pdfCleanStatus == pcNotChecked { //first check
		pdfCleanStatus = pcNothing
		if ConfPdfClean != nil && *ConfPdfClean != "" {
			if _, e := exec.LookPath(*ConfPdfClean); e != nil {
				Log("msg", "no pdfclean (%q) exists?: %v", *ConfPdfClean, e)
			} else {
				pdfCleanStatus = pcPdfClean
			}
		}
		if ConfMutool != nil && *ConfMutool != "" {
			if _, e := exec.LookPath(*ConfMutool); e != nil {
				Log("msg", "no mutool (%q) exists?: %v", *ConfMutool, e)
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
			return errors.Wrapf(err, "clean with "+cleaner)
		}
		cleaned = true
		_, encrypted, _ = pdfPageNum(ctx, fn+"-cleaned.pdf")
		if encrypted {
			Log("msg", "WARN "+cleaner+": file %q is encrypted!", fn)
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
		return errors.Wrapf(err, "%#v while converting %s", cmd, errout.Bytes())
	}
	if len(errout.Bytes()) > 0 {
		Log("msg", "WARN execute %v: %s", cmd, errout.String())
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
		return errors.Wrapf(err, "converting %s to %s with %s",
			srcfn, destfn, *ConfGs)
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
			Log("msg", "ERROR scan: %v", scan.Err())
		}
	}()
	err := cmd.Run()
	if err != nil {
		pw.CloseWithError(err)
		return fields, errors.Wrapf(err, "pdftk generate_fdf")
	}
	pw.Close()
	wg.Wait()
	return fields, nil
}

// PdfDumpFdf dumps the FDF from the given PDF.
func PdfDumpFdf(ctx context.Context, destfn, inpfn string) error {
	if err := call(ctx, *ConfPdftk, inpfn, "generate_fdf", "output", destfn); err != nil {
		return errors.Wrapf(err, "pdftk generate_fdf")
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
			Log("msg", "cannot read fdf %q: %v", fdfFn, fdfErr)
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
		Log("msg", "cannot create %q: %v", fdfFn+".gob", err)
	} else {
		fillFdfMu.Lock()
		err = gob.NewEncoder(f).Encode(fp)
		fillFdfMu.Unlock()
		if err != nil {
			Log("msg", "encode gob %q: %v", f.Name(), err)
		} else {
			if err = f.Close(); err != nil {
				Log("msg", "close %q: %v", f.Name(), err)
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
	Parts  [][]byte
	Fields []string
	Values map[string]string
}

func (fp fieldParts) WriteTo(w io.Writer) (n int64, err error) {
	length := len(fieldPartV)
	fpv1, fpv2 := fieldPartV[:length-2], fieldPartV[length-2:]
	cew := &countErrWriter{w: w}
	length = len(fp.Parts) - 1
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
		Log("msg", "unknown field %q", fp.Fields)
		return errors.New("field " + key + " not exist")
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
