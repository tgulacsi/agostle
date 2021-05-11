// Copyright 2017, 2021 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"context"

	"github.com/tgulacsi/go/i18nmail"
	"github.com/tgulacsi/go/iohlp"

	//"github.com/tgulacsi/go/uncompr"
	"github.com/mholt/archiver"
)

const bodyThreshold = 1 << 20

func MailToPdfZip(ctx context.Context, destfn string, body io.Reader, contentType string) error {
	return MailToSplittedPdfZip(ctx, destfn, body, contentType, false, "", "")
}

type maybeArchItems struct {
	Error error
	Items []ArchFileItem
}

// MailToSplittedPdfZip converts mail to ZIP of PDFs and images
func MailToSplittedPdfZip(ctx context.Context, destfn string, body io.Reader,
	contentType string, split bool, imgmime, imgsize string,
) error {
	Log := getLogger(ctx).Log
	ctx, _ = prepareContext(ctx, "")
	var errs []string
	files, err := MailToPdfFiles(ctx, body, contentType)
	if err != nil {
		fcount := 0
		errs = make([]string, 1, max(1, len(files)))
		errs[0] = err.Error() + "\n"
		for _, f := range files {
			if f.Error == nil {
				fcount++
			} else {
				errs = append(errs, f.Archive+": "+f.Error.Error()+"\n")
			}
		}
		if fcount == 0 {
			return err
		}
	}
	if len(files) == 0 {
		if err == nil {
			err = errors.New("no files to convert")
		}
		return err
	}

	rch := make(chan maybeArchItems, len(files))
	tbz := make([]ArchFileItem, 0, 2*len(files))
	if !split && imgmime == "" {
		tbz = append(tbz, files...)
	} else {
		fts := make([]string, len(files))
		for i, a := range files {
			if a.Error == nil {
				fts[i] = a.Filename
			} else {
				tbz = append(tbz, a)
			}
		}

		go splitPdfMulti(ctx, fts, imgmime, imgsize, rch)
		for ms := range rch {
			if ms.Error != nil {
				errs = append(errs, ms.Error.Error())
			}
			tbz = append(tbz, ms.Items...)
		}
	}

	if len(errs) > 0 {
		Log("msg", "MailToSplittedPdfZip:", "error", errs)
		efn := destfn + "-errors.txt"
		efh, e := os.Create(efn)
		if e != nil {
			Log("msg", "Cannot create errors file", "dest", efn, "error", e)
			return err
		}
		for _, s := range errs {
			if _, e = efh.WriteString(s); e != nil {
				_ = efh.Close()
				Log("msg", "Error writing errors file", "dest", efh.Name(), "error", e)
				return err
			}
		}
		if e = efh.Close(); e != nil {
			Log("msg", "closing errors file", "dest", efh.Name(), "error", e)
		}
		tbz = append(tbz, ArchFileItem{Filename: efn, Archive: ErrTextFn,
			Error: errors.New("")})
	}

	destfh, err := openOut(destfn)
	if err != nil {
		return fmt.Errorf("open out %s: %w", destfn, err)
	}
	ze := ZipFiles(destfh, true, true, []ArchFileItem(ArchItems(tbz).Sort())...)
	if err = destfh.Close(); err != nil && ze == nil {
		ze = err
	}

	cleanupFiles(ctx, files, tbz)

	return ze
}

func cleanupFiles(ctx context.Context, files []ArchFileItem, tbz []ArchFileItem) {
	Log := getLogger(ctx).Log
	_, wd := prepareContext(ctx, "")
	dirs := make(map[string]bool, 16)
	for _, item := range files {
		dirs[filepath.Dir(item.Filename)] = true
		if item.File != nil {
			_ = item.File.Close()
		}
		if !LeaveTempFiles {
			_ = unlink(item.Filename, "after zipped") // ignore error
		}
	}
	for _, item := range tbz {
		dirs[filepath.Dir(item.Filename)] = true
		if item.File != nil {
			_ = item.File.Close()
		}
		if !LeaveTempFiles {
			_ = unlink(item.Filename, "after zipped2") // ignore error
		}
	}
	var (
		dh  *os.File
		fis []os.FileInfo
		n   int
		e   error
	)
	for dn := range dirs {
		if dn == wd {
			continue
		}
		n = 0
		if dh, e = os.Open(dn); e != nil {
			continue
		}
		if fis, e = dh.Readdir(2); e == nil {
			for i := range fis {
				if fis[i].Name() == "doc_data.txt" {
					_ = os.Remove(filepath.Join(dn, fis[i].Name()))
				} else {
					n++
				}
			}
		}
		_ = dh.Close()
		if n == 0 {
			Log("msg", "Removing empty directory", "directory", dn)
			_ = os.Remove(dn)
		}
	}
}

func splitPdfMulti(ctx context.Context, files []string, imgmime, imgsize string, rch chan maybeArchItems) {
	Log := getLogger(ctx).Log
	var sfiles, ifiles, tbd []string
	var err error
	var n int
	mul := 1
	if imgmime != "" {
		mul = 2
	}
	defer func() {
		for _, fn := range tbd {
			_ = unlink(fn, "splitted")
		}
	}()

	for _, fn := range files {
		if !strings.HasSuffix(fn, ".pdf") {
			rch <- maybeArchItems{Items: []ArchFileItem{{Filename: fn}}}
			continue
		}
		sfiles, err = PdfSplit(ctx, fn)
		if err != nil || len(sfiles) == 0 {
			Log("msg", "Splitting", "file", fn, "error", err)
			if err = PdfRewrite(ctx, fn, fn); err != nil {
				Log("msg", "Cannot clean", "file", fn, "error", err)
			} else {
				if sfiles, err = PdfSplit(ctx, fn); err != nil || len(sfiles) == 0 {
					Log("msg", "splitting CLEANED", "file", fn, "error", err)
				}
			}
		}
		if err != nil {
			Log("msg", "splitting", "file", fn, "error", err)
			rch <- maybeArchItems{Error: err}
			continue
		}
		n = mul * len(sfiles)
		items := make([]ArchFileItem, 0, n)
		for _, nm := range sfiles {
			items = append(items, ArchFileItem{Filename: nm})
		}
		if imgmime == "" {
			rch <- maybeArchItems{Items: items}
			continue
		}
		if ifiles, err = PdfToImageMulti(ctx, sfiles, imgmime, imgsize); err != nil {
			Log("msg", "converting to image", "error", err)
		}
		//log.Printf("sfiles=%s err=%s", sfiles, err)
		if !LeaveTempFiles && len(sfiles) > 1 {
			tbd = append(tbd, fn)
		}
		for _, nm := range ifiles {
			items = append(items, ArchFileItem{Filename: nm})
		}
		rch <- maybeArchItems{Items: items}
	}
	close(rch)
}

type pdfToImageArgs struct {
	w, r             *os.File
	name, mime, size string
}

// PdfToImageMulti converts PDF pages to images, using parallel threads
func PdfToImageMulti(ctx context.Context, sfiles []string, imgmime, imgsize string) (imgfilenames []string, err error) {
	if imgmime == "" {
		return
	}
	if imgsize != "" {
		i := strings.Index(imgsize, "x")
		if i < 1 || i >= len(imgsize)-1 {
			return
		}
		if _, err = strconv.Atoi(imgsize[:i]); err != nil {
			return
		}
		if _, err = strconv.Atoi(imgsize[i+1:]); err != nil {
			return
		}
	}
	i := strings.Index(imgmime, "/")
	if i < 0 {
		return
	}
	imgext := "." + imgmime[i+1:]
	imgfilenames = make([]string, 0, len(sfiles))
	imgfnsMtx := sync.Mutex{}
	errs := make([]string, 0, 4)
	errch := make(chan error)
	go func() {
		for e := range errch {
			s := e.Error()
			if s != "" {
				errs = append(errs, s)
			}
		}
	}()
	workch := make(chan pdfToImageArgs)
	var workWg sync.WaitGroup
	work := func() {
		defer ConcLimit.Release(ConcLimit.Acquire())
		defer workWg.Done()
		for args := range workch {
			imErr := PdfToImage(ctx, args.w, args.r, args.mime, args.size)
			if e := args.w.Close(); e != nil && imErr == nil {
				imErr = e
			}
			_ = args.r.Close()
			if imErr != nil {
				errch <- imErr
			} else {
				if fi, err := os.Stat(args.name); err != nil {
					errch <- imErr
				} else if fi.Size() == 0 {
					errch <- fmt.Errorf("%q: zero image", args.name)
				} else {
					imgfnsMtx.Lock()
					imgfilenames = append(imgfilenames, args.name)
					imgfnsMtx.Unlock()
				}
			}
		}
	}
	for i := 0; i < Concurrency; i++ {
		workWg.Add(1)
		go work()
	}
	Log := getLogger(ctx).Log
	for _, sfn := range sfiles {
		rfh, e := os.Open(sfn)
		if e != nil {
			Log("msg", "open PDF for reading", "file", sfn, "error", e)
			errch <- fmt.Errorf("open pdf file %s for reading: %w", sfn, e)
			continue
		}
		//tbz = append(tbz, sfn)
		ifn := sfn + imgext
		ifh, e := os.Create(ifn)
		if e != nil {
			_ = rfh.Close()
			Log("msg", "create image file", "file", sfn, "error", e)
			errch <- fmt.Errorf("create image file %s: %w", ifn, e)
			continue
		}
		workch <- pdfToImageArgs{w: ifh, r: rfh, mime: imgmime, size: imgsize, name: ifn}
	}
	close(workch)
	workWg.Wait()
	close(errch)
	if len(errs) > 0 {
		err = errors.New(strings.Join(errs, "\n"))
	}
	return
}

// SlurpMail splits mail to parts, returns parts and/or error on the given channels
func SlurpMail(ctx context.Context, partch chan<- i18nmail.MailPart, errch chan<- error, body io.Reader, contentType string) {
	Log := getLogger(ctx).Log
	var head [4096]byte

	Log("SlurpMail", contentType)
	mp := i18nmail.MailPart{ContentType: contentType}
	var err error
	if mp.Body, err = i18nmail.MakeSectionReader(body, bodyThreshold); err != nil {
		return
	}
	b := make([]byte, 2048)
	n, _ := mp.Body.ReadAt(b, 0)
	b = b[:n]
	if typ, _ := MIMEMatch(b); typ != "" && !bytes.Contains(b, []byte("\nTo:")) && !bytes.Contains(b, []byte("\nReceived:")) && !bytes.Contains(b, []byte("\nFrom: ")) {
		Log("msg", "not email!", "typ", typ, "ct", contentType)
		if contentType == "" || contentType == "message/rfc822" {
			contentType = typ
		}
		contentType = FixContentType(b, contentType, "")
		Log("msg", "fixed", "contentType", contentType)
		mp.ContentType = contentType
		partch <- mp
		close(partch)
		return
	}
	mp.ContentType = messageRFC822
	err = i18nmail.Walk(
		mp,
		func(mp i18nmail.MailPart) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			fn := headerGetFileName(mp.Header)
			n, err := mp.Body.ReadAt(head[:], 0)
			if err != nil && (!errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) || n == 0) {
				var ok bool
				if _, params, _ := mime.ParseMediaType(
					mp.Header.Get("Content-Disposition"),
				); params != nil {
					s := params["size"]
					if s != "" {
						n, _ = strconv.Atoi(s)
						ok = n <= 64
					}
					Log("msg", "read 0", "size", s, "n", n, "ok", ok)
				}
				if !ok {
					Log("warn", "cannot read", "body", mp, "error", err)
				}
				Log("msg", "SKIP", "Seq", mp.Seq)
				return nil // Skip
			}
			mp.ContentType = FixContentType(head[:n], mp.ContentType, fn)
			_, _ = mp.Body.Seek(0, 0)
			partch <- mp
			return nil
		},
		false)
	if err != nil {
		Log("msg", "Walk finished", "error", err)
		errch <- err
	}
	close(partch)
	//close(errch)
}

const ctxSeen = ctxKey("seen")

// SetupFilters applies filters on parts received on inch, and returns them on outch
func SetupFilters(
	ctx context.Context,
	inch <-chan i18nmail.MailPart, resultch chan<- ArchFileItem,
	errch chan<- error,
) <-chan i18nmail.MailPart {

	if len(Filters) == 0 {
		return inch
	}
	ctx = context.WithValue(ctx, ctxSeen, make(map[string]int, 32))

	in := inch
	var out chan i18nmail.MailPart
	// appending filters
	for _, filter := range Filters {
		out = make(chan i18nmail.MailPart) //new output chan
		go filter(ctx, in, out, resultch, errch)
		in = out
	}
	return out
}

const maxErrLen = 1 << 20

// MailToPdfFiles converts email to PDF files
// all mail part goes through all filter in Filters, in reverse order (last first)
func MailToPdfFiles(ctx context.Context, r io.Reader, contentType string) (files []ArchFileItem, err error) {
	Log := getLogger(ctx).Log
	hsh := sha1.New()
	sr, e := iohlp.MakeSectionReader(io.TeeReader(r, hsh), 1<<20)
	Log("msg", "MailToPdfFiles", "input", sr.Size(), "error", e)
	if e != nil {
		err = fmt.Errorf("MailToPdfFiles: %w", e)
		return
	}

	hshS := base64.URLEncoding.EncodeToString(hsh.Sum(nil))
	ctx, _ = prepareContext(ctx, hshS)
	if _, err = sr.Seek(0, 0); err != nil {
		return nil, err
	}

	files = make([]ArchFileItem, 0, 16)
	errs := make([]string, 0, 16)
	errLen := 0
	resultch := make(chan ArchFileItem)
	rawch := make(chan i18nmail.MailPart)
	errch := make(chan error)

	go SlurpMail(ctx, rawch, errch, sr, contentType) // SlurpMail sends read parts to partch
	partch := SetupFilters(ctx, rawch, resultch, errch)

	// convert parts
	var workWg sync.WaitGroup
	worker := func() {
		defer workWg.Done()
		for mp := range partch {
			if err = convertPart(ctx, mp, resultch); err != nil {
				errch <- err
			}
		}
	}
	for i := 0; i < Concurrency; i++ {
		workWg.Add(1)
		go worker()
	}
	go func() {
		workWg.Wait()
		close(resultch)
	}()

	// collect results and errors
Collect:
	for {
		var ok bool
		var item ArchFileItem
		select {
		case item, ok = <-resultch:
			if !ok {
				close(errch)
				break Collect
			}
			if ok {
				var statter func() (os.FileInfo, error)
				if item.File != nil {
					statter = item.File.Stat
				} else {
					statter = func() (os.FileInfo, error) { return os.Stat(item.Filename) }
				}
				if fi, err := statter(); err != nil {
					errs = append(errs, fmt.Sprintf("stat %q: %+v", item.Filename, err))
				} else if fi.Size() == 0 {
					errs = append(errs, fmt.Sprintf("%q: zero file", item.Filename))
				} else {
					files = append(files, item)
				}
			}
		case err = <-errch:
			if err != nil {
				if errLen < maxErrLen {
					errs = append(errs, err.Error())
					errLen += len(errs[len(errs)-1])
				}
			}
		}
	}

	if err != nil && err != io.EOF {
		errs = append(errs, "error reading parts: "+err.Error())
	}
	if len(errs) > 0 {
		err = errors.New(strings.Join(errs, "\n"))
	}
	return files, err
}

func savePart(ctx context.Context, mp *i18nmail.MailPart) string {
	_, wd := prepareContext(ctx, "")
	return filepath.Join(wd,
		fmt.Sprintf("%02d#%03d.%s", mp.Level, mp.Seq,
			strings.Replace(mp.ContentType, "/", "--", -1)),
	)
}

func convertPart(ctx context.Context, mp i18nmail.MailPart, resultch chan<- ArchFileItem) (err error) {
	Log := getLogger(ctx).Log
	var (
		fn        string
		converter Converter
	)

	fn = savePart(ctx, &mp)

	if messageRFC822 != mp.ContentType {
		converter = GetConverter(mp.ContentType, mp.MediaType)
	} else {
		_, _ = mp.Body.Seek(0, 0)
		plus, e := MailToPdfFiles(ctx, mp.Body, mp.ContentType)
		if e != nil {
			Log("msg", "MailToPdfFiles", "seq", mp.Seq, "error", e)
			err = fmt.Errorf("convertPart(%02d): %w", mp.Seq, e)
			return
		}
		for _, elt := range plus {
			resultch <- elt
		}
		return nil
	}
	if converter == nil { // no converter for this!?
		err = fmt.Errorf("no converter for %s", mp.ContentType)
	} else {
		err = converter(ctx, fn+".pdf", mp.Body, mp.ContentType)
	}
	if err == nil {
		resultch <- ArchFileItem{Filename: fn + ".pdf"}
		return nil
	}
	if errors.Is(err, ErrSkip) {
		return nil
	}
	_ = unlink(fn, "MailToPdfFiles dest part") // ignore error
	Log("msg", "converting to pdf", "ct", mp.ContentType, "fn", fn, "seq", mp.Seq, "error", err)
	j := strings.Index(mp.ContentType, "/")
	_, _ = mp.Body.Seek(0, 0)
	resultch <- ArchFileItem{
		File:    MakeFileLike(mp.Body),
		Archive: mp.ContentType[:j+1] + filepath.Base(fn),
		Error:   err}
	return nil
}

// MailToTree writes mail parts as files starting at outdir as root, trying to reimplement
// the mime hierarchy in the directory hierarchy
func MailToTree(ctx context.Context, outdir string, r io.Reader) error {
	var (
		err    error
		fn, dn string
		ok     bool
		fh     *os.File
	)
	dirs := make(map[int]string, 8)
	dirs[0] = outdir
	up := make([]string, 4)
	upr := make([]string, 4)

	mpName := func(mp i18nmail.MailPart) string {
		xfn := mp.Header.Get("X-FileName")
		if xfn == "" {
			xfn = "eml"
		}
		return fmt.Sprintf("%03d.%s.%s", mp.Seq,
			strings.Replace(mp.ContentType, "/", "--", -1), xfn)
	}

	if err = os.MkdirAll(outdir, 0750); err != nil {
		return fmt.Errorf("MailToTree(%q): %w", outdir, err)
	}
	partch := make(chan i18nmail.MailPart)
	errch := make(chan error, 128)
	go SlurpMail(ctx, partch, errch, r, "")

	for mp := range partch {
		fn = mpName(mp)
		Log("part", mp.Seq, "ct", mp.ContentType)
		dn, ok = dirs[mp.Seq]
		up = up[:0]
		for p := mp.Parent; dn == ""; p = p.Parent {
			if p == nil {
				dn = outdir
				break
			}
			up = append(up, mpName(*p))
			if dn, ok = dirs[p.Seq]; ok {
				upr = upr[:1] // reverse
				upr[0] = dn
				for i := len(up) - 1; i >= 0; i-- {
					upr = append(upr, up[i])
				}
				dn = filepath.Join(upr...)
				_ = os.MkdirAll(dn, 0750)
				break
			}
			//log.Printf("p=%s dn=%s", p, dn)
		}
		if !ok {
			dirs[mp.Seq] = dn
		}

		fn = filepath.Join(dn, fn)
		if fh, err = os.Create(fn); err != nil {
			return fmt.Errorf("create %s: %w", fn, err)
		}
		_, _ = mp.Body.Seek(0, 0)
		if _, err = io.Copy(fh, mp.Body); err != nil {
			_ = fh.Close()
			return fmt.Errorf("read into%s: %w", fn, err)
		}
		if err = fh.Close(); err != nil {
			return fmt.Errorf("close %s: %w", fn, err)
		}
	}
	if err == nil {
		select {
		case err = <-errch:
		default:
		}
	}
	if err != nil && err != io.EOF {
		return fmt.Errorf("MailToTree: %w", err)
	}
	return nil
}

// MailToZip dumps mail and all parts into ZIP
func MailToZip(ctx context.Context, destfn string, body io.Reader, contentType string) error {
	ctx, wd := prepareContext(ctx, "")
	dn, err := ioutil.TempDir(wd, "zip-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dn) }()
	if err = MailToTree(ctx, dn, body); err != nil {
		return err
	}
	destfh, err := openOut(destfn)
	if err != nil {
		return fmt.Errorf("open out %s: %w", destfn, err)
	}
	if err = destfh.Close(); err != nil {
		return err
	}
	ze := ZipTree(destfh, dn, true, true)
	return ze
}

// ExtractingFilter is a filter for the mail pipeline which extracts archives
func ExtractingFilter(ctx context.Context,
	inch <-chan i18nmail.MailPart, outch chan<- i18nmail.MailPart,
	files chan<- ArchFileItem, errch chan<- error,
) {
	Log := getLogger(ctx).Log
	defer func() {
		close(outch)
	}()

	allIn := make(chan i18nmail.MailPart, 1024)
	var wg sync.WaitGroup // for waiting all input to finish
	go func() {
		for part := range inch {
			wg.Add(1)
			allIn <- part
		}
		wg.Wait()
		close(allIn)
	}()
	//seen := make(map[string]struct{})

	for part := range allIn {
		var (
			makeReader   func(io.Reader) (uncomprLister, error)
			zr           uncomprLister
			err          error
			archRowCount int
		)
		body := part.Body
		if part.ContentType == "application/x-ole-storage" || part.ContentType == "application/vnd.ms-outlook" {
			r, oleErr := NewOLEStorageReader(ctx, body)
			if oleErr != nil {
				err = oleErr
				goto Error
			}
			child := part.Spawn()
			child.ContentType = messageRFC822
			if child.Body, err = i18nmail.MakeSectionReader(r, bodyThreshold); err != nil {
				goto Error
			}
			fn := headerGetFileName(part.Header)
			if fn == "" {
				fn = ".eml"
			}
			child.Header = textproto.MIMEHeader(map[string][]string{
				"X-FileName": {safeFn(fn, true)}})
			wg.Add(1)
			allIn <- child
			wg.Done()
			continue
		}

		switch part.ContentType {
		case applicationZIP:
			makeReader = func(r io.Reader) (uncomprLister, error) {
				rsc, err := iohlp.MakeSectionReader(r, 1<<20)
				if err != nil {
					return nil, err
				}
				n, err := rsc.Seek(0, 2)
				if err != nil {
					return nil, err
				}
				if _, err = rsc.Seek(0, 0); err != nil {
					return nil, err
				}
				zp := archiver.NewZip()
				err = zp.Open(rsc, n)
				return zp, err
			}
		case "application/rar":
			makeReader = func(r io.Reader) (uncomprLister, error) {
				rar := archiver.NewRar()
				err := rar.Open(r, 0)
				return rar, err
			}
		//case "application/tar": makeReader = UnTar
		default:
			goto Skip
		}
		zr, err = makeReader(body)
		if err != nil {
			goto Error
		}
		for {
			z, zErr := zr.Read()
			if zErr != nil {
				if zErr == io.EOF {
					break
				}
				Log("msg", "read archive", "error", err)
				break
			}
			if z.Name() == "__MACOSX" {
				Log("msg", "skip", "item", z.Name())
				continue
			}
			if zfh, ok := z.Header.(zip.FileHeader); ok && strings.HasPrefix(zfh.Name, "__MACOSX/") {
				Log("msg", "skip", "item", zfh.Name)
				continue
			}

			archRowCount++
			chunk, cErr := ioutil.ReadAll(z)
			_ = z.Close()
			Log("msg", "read zip element", "i", archRowCount, "fi", z.Name(), "error", err)
			if cErr != nil {
				continue
			}
			child := part.Spawn()
			child.ContentType = FixContentType(chunk, "application/octet-stream",
				z.Name())
			child.Body = io.NewSectionReader(bytes.NewReader(chunk), 0, int64(len(chunk)))
			child.Header = textproto.MIMEHeader(make(map[string][]string, 1))
			child.Header.Add("X-FileName", safeFn(z.Name(), true))
			wg.Add(1)
			allIn <- child
		}
		wg.Done()
		continue
	Error:
		Log("msg", "ExtractingFilter", "ct", part.ContentType, "error", err)
		if err != nil {
			errch <- err
		}
	Skip:
		wg.Done()
		outch <- part
	}
}

func max(x ...int) int {
	a := x[0]
	for i := 1; i < len(x); i++ {
		if a < x[i] {
			a = x[i]
		}
	}
	return a
}

// FilterFunc is the type for the pipeline filter function
// must close out channel on finish!
type FilterFunc func(context.Context, <-chan i18nmail.MailPart, chan<- i18nmail.MailPart, chan<- ArchFileItem, chan<- error)

// Filters is the filter pipeline - order is application order
var Filters = make([]FilterFunc, 0, 6)

func init() {
	Filters = append(Filters, ExtractingFilter)
	Filters = append(Filters, DupFilter)
	Filters = append(Filters, TextDecodeFilter)
	Filters = append(Filters, SaveOriHTMLFilter)
	Filters = append(Filters, PrependHeaderFilter)
	Filters = append(Filters, HTMLPartFilter)
	Filters = append(Filters, DupFilter)
}

func DupFilter(ctx context.Context,
	inch <-chan i18nmail.MailPart, outch chan<- i18nmail.MailPart,
	files chan<- ArchFileItem, errch chan<- error,
) {
	Log := getLogger(ctx).Log
	defer func() {
		close(outch)
	}()
	seen := ctx.Value(ctxSeen).(map[string]int)
	if seen == nil {
		seen = make(map[string]int, 32)
	}
	for part := range inch {
		if hsh := part.Header.Get("X-Hash"); hsh != "" {
			cnt := seen[hsh]
			cnt++
			seen[hsh] = cnt
			if cnt > 10 {
				Log("msg", "DupFilter DROPs", "hash", hsh)
				continue
			}
		}
		outch <- part
	}
}

type uncomprLister interface {
	Read() (archiver.File, error)
}
