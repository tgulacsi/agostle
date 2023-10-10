// Copyright 2017, 2023 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
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
	"github.com/mholt/archiver/v4"
)

const bodyThreshold = 1 << 20

func MailToPdfZip(ctx context.Context, destfn string, body io.Reader, contentType string) error {
	return MailToSplittedPdfZip(ctx, destfn, body, contentType, false, "", "", nil)
}

type maybeArchItems struct {
	Error error
	Items []ArchFileItem
}

// MailToSplittedPdfZip converts mail to ZIP of PDFs and images
func MailToSplittedPdfZip(ctx context.Context, destfn string, body io.Reader,
	contentType string, split bool, imgmime, imgsize string,
	pages []uint16,
) error {
	logger := getLogger(ctx)
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

		go splitPdfMulti(ctx, fts, imgmime, imgsize, rch, pages)
		for ms := range rch {
			if ms.Error != nil {
				errs = append(errs, ms.Error.Error())
			}
			tbz = append(tbz, ms.Items...)
		}
	}

	logger.Info("MailToSplittedPdfZip", "error", errs)
	if len(errs) > 0 {
		efn := destfn + "-errors.txt"
		efh, e := os.Create(efn)
		if e != nil {
			logger.Info("Cannot create errors file", "dest", efn, "error", e)
			return err
		}
		for _, s := range errs {
			if _, e = efh.WriteString(s); e != nil {
				_ = efh.Close()
				logger.Info("Error writing errors file", "dest", efh.Name(), "error", e)
				return err
			}
		}
		if e = efh.Close(); e != nil {
			logger.Info("closing errors file", "dest", efh.Name(), "error", e)
		}
		tbz = append(tbz, ArchFileItem{
			Filename: efn, Archive: ErrTextFn, Error: errors.New(""),
		})
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
	logger := getLogger(ctx)
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
			logger.Info("Removing empty directory", "directory", dn)
			_ = os.Remove(dn)
		}
	}
}

func splitPdfMulti(ctx context.Context, files []string, imgmime, imgsize string, rch chan maybeArchItems, pages []uint16) {
	logger := getLogger(ctx)
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
		sfiles, _, err = PdfSplit(ctx, fn, pages)
		if err != nil || len(sfiles) == 0 {
			logger.Info("Splitting", "file", fn, "error", err)
			if err = PdfRewrite(ctx, fn, fn); err != nil {
				logger.Info("Cannot clean", "file", fn, "error", err)
			} else {
				if sfiles, _, err = PdfSplit(ctx, fn, pages); err != nil || len(sfiles) == 0 {
					logger.Info("splitting CLEANED", "file", fn, "error", err)
				}
			}
		}
		if err != nil {
			logger.Info("splitting", "file", fn, "error", err)
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
			logger.Info("converting to image", "error", err)
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

	type pdfToImageArgs struct {
		w, r             *os.File
		name, mime, size string
	}

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
	logger := getLogger(ctx)
	for _, sfn := range sfiles {
		rfh, e := os.Open(sfn)
		if e != nil {
			logger.Info("open PDF for reading", "file", sfn, "error", e)
			errch <- fmt.Errorf("open pdf file %s for reading: %w", sfn, e)
			continue
		}
		//tbz = append(tbz, sfn)
		ifn := sfn + imgext
		ifh, e := os.Create(ifn)
		if e != nil {
			_ = rfh.Close()
			logger.Info("create image file", "file", sfn, "error", e)
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
	defer close(partch)
	logger := getLogger(ctx).WithGroup("SlurpMail")
	var head [4096]byte

	logger.Info("SlurpMail", "ct", contentType)
	sr, err := i18nmail.MakeSectionReader(body, bodyThreshold)
	if err != nil {
		return
	}
	mp := i18nmail.MailPart{Body: sr, ContentType: contentType}
	b := make([]byte, 2048)
	n, _ := mp.Body.ReadAt(b, 0)
	b = b[:n]
	if typ := MIMEMatch(b); typ != "" &&
		!(bytes.Contains(b, []byte("\nTo:")) || bytes.Contains(b, []byte("\nReceived:")) ||
			bytes.Contains(b, []byte("\nFrom: ")) || bytes.Contains(b, []byte("\nMIME-Version: "))) {
		logger.Info("not email!", "typ", typ, "ct", contentType)
		{
			b := b
			if len(b) > 128 {
				b = b[:128]
			}
			logger.Debug("body", "b", string(b))
		}
		if contentType == "" || contentType == "message/rfc822" {
			contentType = typ
		}
		contentType = FixContentType(b, contentType, "")
		logger.Info("fixed", "contentType", contentType)
		if contentType != messageRFC822 { // sth else
			mp.ContentType = contentType
			partch <- mp
			return
		}
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
			logger := logger.With("level", mp.Level, "seq", mp.Seq)
			fn := headerGetFileName(mp.Header)
			n, err := mp.Body.ReadAt(head[:], 0)
			logger.Info("readAt", "n", n, "error", err, "fn", fn)
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
					logger.Info("read 0", "size", s, "n", n, "ok", ok)
				}
				if !ok {
					logger.Error("cannot read", "body", mp, "error", err)
				}
				logger.Info("SKIP", "Seq", mp.Seq)
				return nil // Skip
			}
			mp.ContentType = FixContentType(head[:n], mp.ContentType, fn)
			_, _ = mp.Body.Seek(0, 0)
			partch <- mp
			return nil
		},
		false)
	if err != nil {
		logger.Info("Walk finished", "error", err)
		errch <- err
	}
	//close(errch)
}

type ctxKeySeen struct{}

// SetupFilters applies filters on parts received on inch, and returns them on outch
func SetupFilters(
	ctx context.Context,
	inch <-chan i18nmail.MailPart, resultch chan<- ArchFileItem,
	errch chan<- error,
) <-chan i18nmail.MailPart {

	if len(Filters) == 0 {
		return inch
	}
	ctx = context.WithValue(ctx, ctxKeySeen{}, make(map[string]int, 32))

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
	logger := getLogger(ctx)
	hsh := sha256.New()
	sr, e := iohlp.MakeSectionReader(io.TeeReader(r, hsh), 1<<20)
	logger.Info("MailToPdfFiles", "input", sr.Size(), "error", e)
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

	if err != nil && !errors.Is(err, io.EOF) {
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
	logger := getLogger(ctx)
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
			logger.Info("MailToPdfFiles", "seq", mp.Seq, "error", e)
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
	logger.Info("converting to pdf", "ct", mp.ContentType, "fn", fn, "seq", mp.Seq, "error", err)
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

	// nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission
	if err = os.MkdirAll(outdir, 0750); err != nil {
		return fmt.Errorf("MailToTree(%q): %w", outdir, err)
	}
	partch := make(chan i18nmail.MailPart)
	errch := make(chan error, 128)
	go SlurpMail(ctx, partch, errch, r, "")

	logger := getLogger(ctx)
	for mp := range partch {
		fn = mpName(mp)
		logger.Info("part", "seq", mp.Seq, "ct", mp.ContentType)
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
				// nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission
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
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("MailToTree: %w", err)
	}
	return nil
}

// MailToZip dumps mail and all parts into ZIP
func MailToZip(ctx context.Context, destfn string, body io.Reader, contentType string) error {
	ctx, wd := prepareContext(ctx, "")
	dn, err := os.MkdirTemp(wd, "zip-")
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
	logger := getLogger(ctx)
	defer func() {
		close(outch)
	}()

	allIn := make(chan i18nmail.MailPart, 1024)
	// nosemgrep: trailofbits.go.waitgroup-add-called-inside-goroutine.waitgroup-add-called-inside-goroutine
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
			format       archiver.Archival
			rsc          *io.SectionReader
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
			body, bodyErr := i18nmail.MakeSectionReader(r, bodyThreshold)
			if err != nil {
				err = bodyErr
				goto Error
			}
			child.Body = body
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
			format = archiver.Zip{}
		case "application/rar":
			format = archiver.Rar{}
		case "application/tar":
			format = archiver.Tar{}
		default:
			goto Skip
		}
		if rsc, err = iohlp.MakeSectionReader(body, 1<<20); err != nil {
			goto Error
		}
		if err = format.Extract(ctx, rsc, nil, func(ctx context.Context, f archiver.File) error {
			name := f.Name()
			if name == "__MACOSX" {
				logger.Info("skip", "item", name)
				return nil
			}
			if zfh, ok := f.Header.(zip.FileHeader); ok && strings.HasPrefix(zfh.Name, "__MACOSX/") {
				logger.Info("skip", "item", zfh.Name)
				return nil
			}

			archRowCount++
			z, err := f.Open()
			if err != nil {
				return err
			}
			chunk, cErr := io.ReadAll(z)
			_ = z.Close()
			logger.Info("read zip element", "i", archRowCount, "fi", name, "error", err)
			if cErr != nil {
				return nil
			}
			child := part.Spawn()
			child.ContentType = FixContentType(chunk, "application/octet-stream", name)
			child.Body = io.NewSectionReader(bytes.NewReader(chunk), 0, int64(len(chunk)))
			child.Header = textproto.MIMEHeader(make(map[string][]string, 1))
			child.Header.Add("X-FileName", safeFn(name, true))
			wg.Add(1)
			allIn <- child
			return nil
		}); err != nil {
			goto Error
		}
		wg.Done()
		continue
	Error:
		logger.Info("ExtractingFilter", "ct", part.ContentType, "error", err)
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
	logger := getLogger(ctx)
	defer func() {
		close(outch)
	}()
	seen := ctx.Value(ctxKeySeen{}).(map[string]int)
	if seen == nil {
		seen = make(map[string]int, 32)
	}
	for part := range inch {
		if hsh := part.Header.Get("X-Hash"); hsh != "" {
			cnt := seen[hsh]
			cnt++
			seen[hsh] = cnt
			if cnt > 10 {
				logger.Info("DupFilter DROPs", "hash", hsh)
				continue
			}
		}
		outch <- part
	}
}
