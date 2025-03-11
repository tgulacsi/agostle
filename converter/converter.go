// Copyright 2019, 2023 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"context"

	"github.com/UNO-SOFT/filecache"
	"github.com/mholt/archives"
	"github.com/tgulacsi/go/iohlp"
	"golang.org/x/net/html"
)

var ErrSkip = errors.New("skip this part")

// Converter converts to Pdf (destination filename, source reader and source content-type)
type Converter func(context.Context, string, io.Reader, string) error

func (c Converter) WithCache(ctx context.Context, destfn string, r io.Reader, sourceContentType, destContentType string) error {
	logger := getLogger(ctx).With("f", "convertWithCache", "sct", sourceContentType, "dct", destContentType, "dest", destfn)
	hsh := filecache.NewHash()
	if destContentType == "" {
		destContentType = "application/pdf"
	}
	hsh.Write([]byte(sourceContentType + ":" + destContentType + ":"))
	ifh, ok := r.(*os.File)
	if ok && fileExists(ifh.Name()) {
		if _, err := io.Copy(hsh, ifh); err != nil {
			return err
		}
		if _, err := ifh.Seek(0, 0); err != nil {
			return err
		}
	} else {
		var err error
		typ := sourceContentType
		if i := strings.IndexByte(typ, '/'); i >= 0 {
			typ = typ[i+1:]
		}
		if i := strings.IndexAny(typ, "; "); i >= 0 {
			typ = typ[:i]
		}
		inpfn := destfn + "." + typ
		ifh, err = os.Create(inpfn)
		if err != nil {
			return fmt.Errorf("create temp image file %s: %w", inpfn, err)
		}
		if _, err = io.Copy(io.MultiWriter(ifh, hsh), r); err != nil {
			logger.Info("reading", "file", ifh.Name(), "error", err)
		}
		if err = ifh.Close(); err != nil {
			logger.Info("writing", "dest", ifh.Name(), "error", err)
		}
		if ifh, err = os.Open(inpfn); err != nil {
			return fmt.Errorf("open inp %s: %w", inpfn, err)
		}
		defer func() { _ = ifh.Close() }()
		if !LeaveTempFiles {
			defer func() { _ = unlink(inpfn, "convertWithCache") }()
		}
	}

	key := filecache.ActionID(hsh.SumID())
	if fn, _, err := Cache.GetFile(key); err == nil {
		if err = copyFile(fn, destfn); err == nil {
			logger.Info("served from cache")
			return nil
		}
		logger.Info("copy from cache", "source", fn, "dest", destfn, "error", err)
	}

	if _, err := ifh.Seek(0, 0); err != nil {
		return err
	}
	if err := c(ctx, destfn, ifh, sourceContentType); err != nil {
		return err
	}

	ofh, err := os.Open(destfn)
	if err != nil {
		return err
	}

	_, _, err = Cache.Put(key, ofh)
	ofh.Close()
	logger.Info("store into cache", "dest", ofh.Name(), "key", hex.EncodeToString(key[:]), "error", err)
	return nil
}

// TextToPdf converts text (text/plain) to PDF
func TextToPdf(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	getLogger(ctx).Info("Converting into", "ct", contentType, "dest", destfn)
	return HTMLToPdf(ctx, destfn, textToHTML(r), textHtml)
}

func textToHTML(r io.Reader) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		if _, err := io.Copy(&htmlEscaper{w: pw}, iohlp.WrappingReader(r, 80)); err != nil {
			logger.Info("escape", "error", err)
			_ = pw.CloseWithError(err)
			return
		}
		pw.Close()
	}()
	return io.MultiReader(
		strings.NewReader(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body><pre>`),
		pr,
		strings.NewReader("</pre></body></html>"),
	)
}

// ImageToPdf convert image (image/...) to PDF
func ImageToPdf(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	return Converter(imageToPdf).WithCache(ctx, destfn, r, contentType, "application/pdf")
}
func imageToPdf(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	logger := getLogger(ctx)
	logger.Info("converting image", "ct", contentType, "dest", destfn)
	imgtyp := contentType[strings.Index(contentType, "/")+1:]
	destfn = strings.TrimSuffix(destfn, ".pdf")

	ifh, ok := r.(*os.File)
	if !ok && fileExists(ifh.Name()) {
		if _, err := ifh.Seek(0, 0); err != nil {
			return err
		}
	} else {
		inpfn := destfn + "." + imgtyp
		var err error
		if contentType == "image/heic" {
			imgtyp, inpfn = "jpeg", destfn+".jpeg"
			var buf strings.Builder
			cmd := command(ctx, *ConfGm, "convert", "-", inpfn)
			cmd.Stdin = r
			cmd.Stderr = &buf
			if err = cmd.Run(); err != nil {
				return fmt.Errorf("convert heic to %s: %w: %s", imgtyp, err, buf.String())
			}
		} else {
			ifh, err = os.Create(inpfn)
			if err != nil {
				return fmt.Errorf("create temp image file %s: %w", inpfn, err)
			}
			if _, err = io.Copy(ifh, r); err != nil {
				logger.Info("ImageToPdf reading", "file", ifh.Name(), "error", err)
			}
			if err = ifh.Close(); err != nil {
				logger.Info("ImageToPdf writing", "dest", ifh.Name(), "error", err)
			}
		}
		if ifh, err = os.Open(inpfn); err != nil {
			return fmt.Errorf("open inp %s: %w", inpfn, err)
		}
		defer func() { _ = ifh.Close() }()
		if !LeaveTempFiles {
			defer func() { _ = unlink(inpfn, "ImageToPdf") }()
		}
	}
	destfn = destfn + ".pdf"
	w, err := os.Create(destfn)
	if err != nil {
		return err
	}
	defer w.Close()

	logger.Info("ImageToPdfPdfCPU")
	if err = ImageToPdfPdfCPU(w, ifh); err != nil {
		logger.Info("imageToPdfPdfCPU", "error", err)
		if _, seekErr := ifh.Seek(0, 0); seekErr != nil {
			return seekErr
		}
		if _, seekErr := w.Seek(0, 0); seekErr != nil {
			return seekErr
		}
		if *ConfGm != "" {
			return err
		}
		logger.Info("ImageToPdfGm")
		if err = ImageToPdfGm(ctx, w, ifh, contentType); err != nil {
			logger.Info("ImageToPdfGm", "error", err)
		}
	}
	if closeErr := w.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

// OfficeToPdf converts other to PDF with LibreOffice
func OfficeToPdf(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	return Converter(officeToPdf).WithCache(ctx, destfn, r, contentType, "application/pdf")
}
func officeToPdf(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	getLogger(ctx).Info("Converting into", "ct", contentType, "dest", destfn)
	destfn = strings.TrimSuffix(destfn, ".pdf")
	inpfn := destfn + ".raw"
	fh, err := os.Create(inpfn)
	if err != nil {
		return err
	}
	defer func() { _ = unlink(inpfn, "OtherToPdf") }()
	if _, err = io.Copy(fh, r); err != nil {
		return err
	}
	return lofficeConvert(ctx, filepath.Dir(destfn), inpfn, contentType)
}

// OtherToPdf is the default converter
var OtherToPdf = OfficeToPdf

// PdfToPdf "converts" PDF (application/pdf) to PDF (just copies)
func PdfToPdf(ctx context.Context, destfn string, r io.Reader, _ string) error {
	getLogger(ctx).Info(`"Converting" pdf into`, "dest", destfn)
	fh, err := os.Create(destfn)
	if err != nil {
		return err
	}
	_, err = io.Copy(fh, r)
	closeErr := fh.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// MPRelatedToPdf converts multipart/related to PDF
func MPRelatedToPdf(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	//Log := getLogger(ctx).Log
	var (
		err    error
		params map[string]string
	)
	contentType, params, err = mime.ParseMediaType(contentType)
	if err != nil {
		err = fmt.Errorf("parse Content-Type %s: %w", contentType, err)
		return err
	}

	parts := multipart.NewReader(r, params["boundary"])
	_, e := parts.NextPart()
	for e == nil {
		_, e = parts.NextPart()
	}
	if e != nil && !errors.Is(e, io.EOF) {
		return e
	}
	return nil
}

// HTMLToPdf converts HTML (text/html) to PDF
func HTMLToPdf(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	return Converter(htmlToPdf).WithCache(ctx, destfn, r, contentType, "application/pdf")
}
func htmlToPdf(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	logger := getLogger(ctx).With("func", "HTMLToPdf", "dest", destfn)
	var inpfn string
	if fh, ok := r.(*os.File); ok && fileExists(fh.Name()) {
		inpfn = fh.Name()
	}
	if inpfn == "" {
		inpfn = nakeFilename(destfn) + ".html"
		fh, err := os.Create(inpfn)
		if err != nil {
			return err
		}
		if !LeaveTempFiles {
			defer func() { _ = unlink(inpfn, "HtmlToPdf") }()
		}
		if _, err = io.Copy(fh, r); err != nil {
			return err
		}
	} else {

		b, err := os.ReadFile(inpfn)
		if err == nil {
			var f func(*html.Node) *html.Node
			f = func(n *html.Node) *html.Node {
				if n == nil || n.Type == html.ElementNode && n.Data == "img" {
					return n
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if n := f(c); n != nil {
						return n
					}
				}
				return nil
			}
			var buf bytes.Buffer
			for _, pos := range reHtmlImg.FindAllIndex(b, -1) {
				line := b[pos[0]:pos[1]]
				img, _ := html.Parse(bytes.NewReader(line))
				if img = f(img); img == nil {
					continue
				}
				// delete height, modify width
				for i := 0; i < len(img.Attr); i++ {
					switch strings.ToLower(img.Attr[i].Key) {
					case "height", "style":
						img.Attr[i] = img.Attr[0]
						img.Attr = img.Attr[1:]
						i--
					case "width":
						if len(img.Attr[i].Val) > 3 {
							img.Attr[i].Val = "100%"
						}
					case "src":
						if !*ConfKeepRemoteImage {
							if s := img.Attr[i].Val; strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") {
								img.Attr[i] = img.Attr[0]
								img.Attr = img.Attr[1:]
								i--
							}
						}
					}
				}
				buf.Reset()
				if err = html.Render(&buf, img); err != nil {
					logger.Info("html.Render", "img", img, "error", err)
					continue
				}
				logger.Info("htmlToPdf", "old", string(line), "new", buf.String())
				i := pos[0] + copy(b[pos[0]:pos[1]], buf.Bytes())
				for i < pos[1] {
					b[i] = ' '
					i++
				}
			}

			if err = os.WriteFile(inpfn, b, 0644); err != nil {
				return fmt.Errorf("overwrite %s: %w", inpfn, err)
			}
		}
	}

	if gotenberg.Valid() {
		err := gotenberg.PostFileNames(ctx, destfn, "/forms/chromium/convert/html", []string{inpfn}, "text/html")
		if err == nil {
			return nil
		}
		logger.Debug("gotenberg chromium", "error", err)
	}

	if *ConfWkhtmltopdf != "" {
		err := wkhtmltopdf(ctx, destfn, inpfn)
		if err == nil {
			return nil
		}
		logger.Info("wkhtmltopdf", "error", err)
	}

	dn := filepath.Dir(destfn)
	outfn := filepath.Join(dn, filepath.Base(nakeFilename(inpfn))+".pdf")
	if err := lofficeConvert(ctx, dn, inpfn, contentType); err != nil {
		return err
	}
	if outfn != destfn {
		return moveFile(outfn, destfn)
	}
	return nil
}

func OutlookToEML(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	return Converter(outlookToEML).WithCache(ctx, destfn, r, contentType, messageRFC822)
}
func outlookToEML(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	rc, err := NewOLEStorageReader(ctx, r)
	logger.Info("OutlookToEML", "ct", contentType, "error", err)
	if err != nil {
		return err
	}
	defer rc.Close()
	return Converter(MailToPdfZip).WithCache(ctx, destfn, rc, messageRFC822, messageRFC822)
}

var reHtmlImg = regexp.MustCompile(`(?i)(<img[^>]*/?>)`)

// Skip skips the conversion
func Skip(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	return ErrSkip
}

var (
	lofficeMu       = sync.Mutex{}
	lofficePortLock = NewPortLock(LofficeLockPort)
)

// calls loffice converter with only one instance at a time,
// in the input file's directory
func lofficeConvert(ctx context.Context, outDir, inpfn, contentType string) error {
	if outDir == "" {
		return errors.New("outDir is required")
	}
	logger := getLogger(ctx)
	if gotenberg.Valid() {
		err := gotenberg.PostFileNames(ctx, filepath.Join(outDir, filepath.Base(inpfn)+".pdf"), "/forms/libreoffice/convert", []string{inpfn}, contentType)
		if err == nil {
			return nil
		}
		logger.Debug("libreofficeConvert gotenberg", "error", err)
	}
	args := []string{"--headless", "--convert-to", "pdf", "--outdir",
		outDir, inpfn}
	lofficeMu.Lock()
	defer lofficeMu.Unlock()
	if lofficePortLock != nil {
		lofficePortLock.Lock()
		defer lofficePortLock.Unlock()
	}
	subCtx, subCancel := context.WithTimeout(ctx, *ConfLofficeTimeout)
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := Exec.CommandContext(subCtx, *ConfLoffice, args...)
	cmd.Dir = filepath.Dir(inpfn)
	cmd.Stderr = os.Stderr
	cmd.Stdout = cmd.Stderr
	if runtime.GOOS != "windows" {
		// This induces "soffice.exe: The parameter is incorrect." error under Windows!
		cmd.Env = make([]string, 1, len(os.Environ())+1)
		lcAll := os.Getenv("LC_ALL")
		if i := strings.IndexByte(lcAll, '.'); i > 0 && strings.HasPrefix(lcAll, "en_") {
			lcAll = lcAll[:i+1] + "UTF-8"
		} else {
			lcAll = "en_US.UTF-8"
		}
		cmd.Env[0] = lcAll
		logger.Info("env LC_ALL=" + lcAll)
		// delete LC_* LANG* env vars.
		for _, s := range os.Environ() {
			if strings.HasPrefix(s, "LC_") || s == "LANG" || s == "LANGUAGE" {
				continue
			}
			cmd.Env = append(cmd.Env, s)
		}
	}

	err := cmd.Run()
	subCancel()
	if err != nil {
		return fmt.Errorf("%q: %w", cmd.Args, err)
	}
	outfn := filepath.Join(outDir, filepath.Base(nakeFilename(inpfn))+".pdf")
	if _, err := os.Stat(outfn); err != nil {
		return fmt.Errorf("%v no output for %s: %w", cmd.Args, filepath.Base(inpfn), err)
	}
	return nil
}

// calls wkhtmltopdf
func wkhtmltopdf(ctx context.Context, outfn, inpfn string) error {
	logger := getLogger(ctx)
	ussFh, err := os.CreateTemp("", "uss-*.css")
	if err != nil {
		return err
	}
	defer ussFh.Close()
	ussFn := ussFh.Name()
	defer func() { _ = os.Remove(ussFn) }()
	if _, err = ussFh.Write([]byte(`pre {
	white-space: pre-line;
}`)); err != nil {
		return err
	}
	if err = ussFh.Close(); err != nil {
		return err
	}

	args := []string{
		"-s", "A4",
		inpfn,
		"--allow", "images",
		"--encoding", "utf-8",
		"--load-error-handling", "ignore",
		"--load-media-error-handling", "ignore",
		"--images",
		"--enable-local-file-access",
		"--no-background",
		"--user-style-sheet", ussFn,
		outfn}
	var buf bytes.Buffer
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := Exec.CommandContext(ctx, *ConfWkhtmltopdf, args...)
	cmd.Dir = filepath.Dir(inpfn)
	cmd.Stderr = &buf
	cmd.Stdout = os.Stdout
	logger.Info("start wkhtmltopdf", "args", cmd.Args)
	if err := cmd.Run(); err != nil {
		err = fmt.Errorf("%q: %w", cmd.Args, err)
		if bytes.HasSuffix(buf.Bytes(), []byte("ContentNotFoundError\n")) ||
			bytes.HasSuffix(buf.Bytes(), []byte("ProtocolUnknownError\n")) ||
			bytes.HasSuffix(buf.Bytes(), []byte("HostNotFoundError\n")) { // K-MT11422:99503
			logger.Info(buf.String())
		} else {
			return fmt.Errorf("%s: %w", buf.String(), err)
		}
	}
	if fi, err := os.Stat(outfn); err != nil {
		return fmt.Errorf("wkhtmltopdf no output for %s: %w", filepath.Base(inpfn), err)
	} else if fi.Size() == 0 {
		return fmt.Errorf("wkhtmltopdf empty output for %s", filepath.Base(inpfn))
	}
	return nil
}

// file extension -> content-type map
var ExtContentType = map[string]string{
	"doc":  "application/vnd.ms-word",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"dotx": "application/vnd.openxmlformats-officedocument.wordprocessingml.template",
	"xls":  "application/vnd.ms-excel",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"xltx": "application/vnd.openxmlformats-officedocument.spreadsheetml.template",
	"ppt":  "application/vnd.ms-powerpoint",
	"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"ppsx": "application/vnd.openxmlformats-officedocument.presentationml.slideshow",
	"potx": "application/vnd.openxmlformats-officedocument.presentationml.template",

	"odg": "application/vnd.oasis.opendocument.graphics",
	"otg": "application/vnd.oasis.opendocument.graphics-template",
	"otp": "application/vnd.oasis.opendocument.presentation-template",
	"odp": "application/vnd.oasis.opendocument.presentation",
	"odm": "application/vnd.oasis.opendocument.text-master",
	"odt": "application/vnd.oasis.opendocument.text",
	"oth": "application/vnd.oasis.opendocument.text-web",
	"ott": "application/vnd.oasis.opendocument.text-template",
	"ods": "application/vnd.oasis.spreadsheet",
	"ots": "application/vnd.oasis.spreadsheet-template",
	"odc": "application/vnd.oasis.chart",
	"odf": "application/vnd.oasis.formula",
	"odb": "application/vnd.oasis.database",
	"odi": "application/vnd.oasis.image",

	"txt": textPlain,
	"msg": mimeOutlook,

	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"gif":  "image/gif",
	"png":  "image/png",
	"tif":  "image/tif",
	"tiff": "image/tiff",

	"es3": "text/xades+xml",
}

const mimeOutlook = "application/vnd.ms-outlook"

func fixCT(contentType, fileName string) (ct string) {
	//defer func() {
	//	logger.Info("fixCT", "ct", contentType, "fn", fileName, "result", ct)
	//}()

	switch contentType {
	case "application/CDFV2":
		return mimeOutlook
	case "application/msword":
		return "application/vnd.ms-word"
	case applicationZIP, "application/x-zip-compressed":
		if ext := filepath.Ext(fileName); len(ext) > 3 {
			// http://www.iana.org/assignments/media-types/media-types.xhtml#application
			switch ext {
			case ".docx", ".xlsx", ".pptx", ".ods", ".odt", ".odp":
				return ExtContentType[ext[1:]]
			}
		}
		return applicationZIP
	case "application/x-rar-compressed", "application/x-rar":
		return "application/rar"
	case "image/pdf":
		return applicationPDF
	case "text/xml":
		if ext := filepath.Ext(fileName); len(ext) > 3 && ext == ".es3" {
			return "text/xades+xml"
		}
	}
	return contentType
}

// FixContentType ensures proper content-type
// (uses magic for "" and application/octet-stream)
func FixContentType(body []byte, contentType, fileName string) (ct string) {
	where := "0"
	ext := strings.ToLower(filepath.Ext(fileName))
	defer func() {
		if contentType != ct {
			logger.Info("FixContentType", "ct", contentType, "fn", fileName, "ext", ext, "result", ct, "where", where)
		}
	}()
	if bytes.HasPrefix(body, []byte("<?xml version=\"1.0\"")) {
		contentType = "text/xml"
		if bytes.Contains(body, []byte("https://www.microsec.hu/ds/e-szigno3")) {
			contentType = "text/es3+xml"
		} else if bytes.Contains(body, []byte("http://uri.etsi.org/01903/")) {
			contentType = "text/xades+xml"
		}
		return contentType
	}

	contentType = fixCT(contentType, fileName)
	if strings.HasPrefix(ext, ".") {
		if want, ok := ExtContentType[ext[1:]]; ok && contentType != want {
			if typ := MIMEMatch(body); typ != "" && typ != contentType {
				where = "A"
				return fixCT(typ, fileName)
			}
		}
	}
	c := GetConverter(contentType, nil)
	if c == nil { // no converter for this
		if typ := MIMEMatch(body); typ != "" && typ != contentType {
			where = "B"
			return fixCT(typ, fileName)
		}
	}
	if fileName != "" &&
		(contentType == "" || contentType == "application/octet-stream" || c == nil) {
		if len(ext) > 3 {
			if nct, ok := ExtContentType[ext[1:]]; ok {
				where = "C"
				return fixCT(nct, fileName)
			}
			if nct := mime.TypeByExtension(ext); nct != "" {
				where = "D"
				return fixCT(nct, fileName)
			}
		}
	}
	//log.Printf("ct=%s ==> %s", ct, contentType)
	where = "E"
	return contentType
}

const (
	textHtml       = "text/html"
	textPlain      = "text/plain"
	applicationPDF = "application/pdf"
	applicationZIP = "application/zip"
	messageRFC822  = "message/rfc822"
)

// GetConverter gets converter for the content-type
func GetConverter(contentType string, mediaType map[string]string) (converter Converter) {
	converter = nil
	switch contentType {
	case applicationPDF:
		converter = PdfToPdf
	case "application/rtf":
		converter = OfficeToPdf
	case textPlain:
		if mediaType != nil {
			if cs, ok := mediaType["charset"]; ok && cs != "" {
				converter = NewTextConverter(cs)
			}
		}
		if converter == nil {
			converter = TextToPdf
		}
	case textHtml:
		converter = HTMLToPdf
	case messageRFC822:
		converter = MailToPdfZip
	case mimeOutlook, "application/CDFV2":
		converter = OutlookToEML
	case "multipart/related":
		converter = MPRelatedToPdf
	case applicationZIP:
		converter = Decompress
	case "text/es3+xml":
		converter = Decompress
	case "application/x-pkcs7-signature", "text/xml":
		converter = Skip
	default:
		if strings.HasPrefix(contentType, "text/") && strings.HasSuffix(contentType, "+xml") {
			converter = Skip
			break
		}
		// from http://www.openoffice.org/framework/documentation/mimetypes/mimetypes.html
		if strings.HasPrefix(contentType, "application/vnd.oasis.") ||
			//ODF
			strings.HasPrefix(contentType, "application/vnd.openxmlformats-officedocument.") ||
			//MS Office
			strings.HasPrefix(contentType, "application/vnd.ms-word") ||
			strings.HasPrefix(contentType, "application/vnd.ms-excel") ||
			strings.HasPrefix(contentType, "application/vnd.ms-powerpoint") ||
			contentType == "application/x-ole-storage" ||
			//StarOffice
			strings.HasPrefix(contentType, "application/vnd.sun.xml.") ||
			strings.HasPrefix(contentType, "application/vnd.stardivision.") ||
			strings.HasPrefix(contentType, "application/x-star.") ||
			//Word
			contentType == "application/msword" {
			converter = OfficeToPdf
			break
		}
		i := strings.Index(contentType, "/")
		if i > 0 {
			switch contentType[:i] {
			case "image":
				converter = ImageToPdf
			case "text":
				converter = TextToPdf
			case "audio", "video":
				converter = nil
			}
		}
	}
	return
}

func Decompress(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	toPDF := func(ctx context.Context, pdfs []string, r io.Reader, contentType string) ([]string, error) {
		next := GetConverter(contentType, nil)
		if next == nil {
			logger.Warn("no converter for", "ct", contentType)
			return pdfs, nil
		}
		tempFh, err := os.CreateTemp(
			filepath.Dir(destfn),
			strings.TrimSuffix(filepath.Base(destfn), ".pdf")+"-*.pdf",
		)
		if err != nil {
			return pdfs, err
		}
		tempFh.Close()
		if err := next(ctx, tempFh.Name(), r, contentType); err != nil {
			return pdfs, err
		}
		pdfs = append(pdfs, tempFh.Name())
		return pdfs, nil
	}
	var pdfs []string
	defer func() {
		for _, fn := range pdfs {
			os.Remove(fn)
		}
	}()
	if contentType == applicationZIP {
		archives.Zip{}.Extract(ctx, r, func(ctx context.Context, f archives.FileInfo) error {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			var a [1024]byte
			n, _ := io.ReadAtLeast(rc, a[:], 512)
			body := a[:n]
			r := io.MultiReader(bytes.NewReader(body), rc)
			pdfs, err = toPDF(ctx, pdfs, r, FixContentType(body, "", f.Name()))
			return err

		})
	} else if contentType == "text/es3+xml" {
		dec := xml.NewDecoder(r)
		var x es3Dossier
		if err := dec.Decode(&x); err != nil {
			return fmt.Errorf("decode as es3: %w", err)
		}
		for _, d := range x.Documents.Document {
			logger.Info("found es3 document", "profile", d.DocumentProfile)
			r := io.Reader(bytes.NewReader(d.Object.Data))
			if d.DocumentProfile.BaseTransform.Transform.Algorithm == "base64" {
				r = base64.NewDecoder(base64.StdEncoding, r)
			}
			ct := d.DocumentProfile.Format.MIMEType.Type + "/" + d.DocumentProfile.Format.MIMEType.Subtype
			var err error
			if pdfs, err = toPDF(ctx, pdfs, r, ct); err != nil {
				return fmt.Errorf("sub object %+v: %w", d.DocumentProfile, err)
			}
		}
	} else {
		next := GetConverter(contentType, nil)
		if next != nil {
			return next(ctx, destfn, r, contentType)
		}
	}
	return pdfMerge(ctx, destfn, pdfs...)
}

type (
	es3Dossier struct {
		XMLName        xml.Name   `xml:"Dossier"`
		DossierProfile es3Profile `xml:"DossierProfile"`
		Documents      struct {
			ID       string        `xml:"Id,attr"`
			Document []es3Document `xml:"Document"`
		} `xml:"Documents"`
		Signature struct {
			ID         string `xml:"Id,attr"`
			SignedInfo struct {
				ID                     string `xml:"Id,attr"`
				CanonicalizationMethod struct {
					Algorithm string `xml:"Algorithm,attr"`
				} `xml:"CanonicalizationMethod"`
				SignatureMethod struct {
					Algorithm string `xml:"Algorithm,attr"`
				} `xml:"SignatureMethod"`
				Reference []struct {
					ID         string `xml:"Id,attr"`
					URI        string `xml:"URI,attr"`
					Type       string `xml:"Type,attr"`
					Transforms struct {
						Transform struct {
							Algorithm string `xml:"Algorithm,attr"`
						} `xml:"Transform"`
					} `xml:"Transforms"`
					DigestMethod struct {
						Algorithm string `xml:"Algorithm,attr"`
					} `xml:"DigestMethod"`
					DigestValue string `xml:"DigestValue"`
				} `xml:"Reference"`
			} `xml:"SignedInfo"`
			SignatureValue struct {
				ID string `xml:"Id,attr"`
			} `xml:"SignatureValue"`
			KeyInfo struct {
				ID       string `xml:"Id,attr"`
				X509Data struct {
					X509Certificate string `xml:"X509Certificate"`
				} `xml:"X509Data"`
			} `xml:"KeyInfo"`
			Object []struct {
				ID               string `xml:"Id,attr"`
				SignatureProfile struct {
					ID          string `xml:"Id,attr"`
					ObjRef      string `xml:"OBJREF,attr"`
					SigRef      string `xml:"SIGREF,attr"`
					SigRefList  string `xml:"SIGREFLIST,attr"`
					SignerName  string `xml:"SignerName"`
					SDPresented string `xml:"SDPresented"`
					Type        string `xml:"Type"`
					Generator   struct {
						Program struct {
							Name    string `xml:"name,attr"`
							Version string `xml:"version,attr"`
						} `xml:"Program"`
						Device struct {
							Name string `xml:"name,attr"`
							Type string `xml:"type,attr"`
						} `xml:"Device"`
					} `xml:"Generator"`
				} `xml:"SignatureProfile"`
				QualifyingProperties struct {
					Target           string `xml:"Target,attr"`
					ID               string `xml:"Id,attr"`
					SignedProperties struct {
						ID                        string `xml:"Id,attr"`
						SignedSignatureProperties struct {
							SigningTime          string `xml:"SigningTime"`
							SigningCertificateV2 struct {
								Cert struct {
									CertDigest struct {
										DigestMethod struct {
											Algorithm string `xml:"Algorithm,attr"`
										} `xml:"DigestMethod"`
										DigestValue string `xml:"DigestValue"`
									} `xml:"CertDigest"`
									IssuerSerialV2 string `xml:"IssuerSerialV2"`
								} `xml:"Cert"`
							} `xml:"SigningCertificateV2"`
							SignaturePolicyIdentifier struct {
								SignaturePolicyImplied string `xml:"SignaturePolicyImplied"`
							} `xml:"SignaturePolicyIdentifier"`
							SignerRoleV2 struct {
								ClaimedRoles struct {
									ClaimedRole string `xml:"ClaimedRole"`
								} `xml:"ClaimedRoles"`
							} `xml:"SignerRoleV2"`
						} `xml:"SignedSignatureProperties"`
					} `xml:"SignedProperties"`
					UnsignedProperties struct {
						UnsignedSignatureProperties struct {
							SignatureTimeStamp struct {
								ID                     string `xml:"Id,attr"`
								CanonicalizationMethod struct {
									Algorithm string `xml:"Algorithm,attr"`
								} `xml:"CanonicalizationMethod"`
								EncapsulatedTimeStamp struct {
									ID string `xml:"Id,attr"`
								} `xml:"EncapsulatedTimeStamp"`
							} `xml:"SignatureTimeStamp"`
							CertificateValues struct {
								ID                          string `xml:"Id,attr"`
								EncapsulatedX509Certificate []struct {
									ID string `xml:"Id,attr"`
								} `xml:"EncapsulatedX509Certificate"`
							} `xml:"CertificateValues"`
						} `xml:"UnsignedSignatureProperties"`
					} `xml:"UnsignedProperties"`
				} `xml:"QualifyingProperties"`
			} `xml:"Object"`
		} `xml:"Signature"`
	}

	es3Profile struct {
		ID           string `xml:"Id,attr"`
		ObjRef       string `xml:"OBJREF,attr"`
		Title        string `xml:"Title"`
		ECategory    string `xml:"E-category"`
		CreationDate string `xml:"CreationDate"`
	}
	es3Document struct {
		DocumentProfile struct {
			ID           string `xml:"Id,attr"`
			ObjRef       string `xml:"OBJREF,attr"`
			Title        string `xml:"Title"`
			CreationDate string `xml:"CreationDate"`
			Format       struct {
				MIMEType struct {
					Type      string `xml:"type,attr"`
					Subtype   string `xml:"subtype,attr"`
					Extension string `xml:"extension,attr"`
				} `xml:"MIME-Type"`
			} `xml:"Format"`
			SourceSize struct {
				SizeValue string `xml:"sizeValue,attr"`
				SizeUnit  string `xml:"sizeUnit,attr"`
			} `xml:"SourceSize"`
			BaseTransform struct {
				Transform struct {
					Algorithm string `xml:"Algorithm,attr"`
				} `xml:"Transform"`
			} `xml:"BaseTransform"`
		} `xml:"DocumentProfile"`
		Object struct {
			ID   string `xml:"Id,attr"`
			Data []byte `xml:",chardata"`
		} `xml:"Object"`
	}
)
