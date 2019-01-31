// Copyright 2019 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bytes"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"context"

	"github.com/go-kit/kit/log"
	"github.com/h2non/filetype"
	"github.com/h2non/filetype/types"
	"github.com/pkg/errors"
	"github.com/tgulacsi/go/iohlp"
	"golang.org/x/net/html"
)

var ErrSkip = errors.New("skip this part")

// Converter converts to Pdf (destination filename, source reader and source content-type)
type Converter func(context.Context, string, io.Reader, string) error

// TextToPdf converts text (text/plain) to PDF
func TextToPdf(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	getLogger(ctx).Log("msg", "Converting into", "ct", contentType, "dest", destfn)
	return HTMLToPdf(ctx, destfn, textToHTML(r), textHtml)
}

func textToHTML(r io.Reader) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		if _, err := io.Copy(&htmlEscaper{w: pw}, iohlp.WrappingReader(r, 80)); err != nil {
			Log("msg", "escape", "error", err)
			pw.CloseWithError(err)
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
	Log := getLogger(ctx).Log
	Log("msg", "converting image", "ct", contentType, "dest", destfn)
	imgtyp := contentType[strings.Index(contentType, "/")+1:]
	if strings.HasSuffix(destfn, ".pdf") {
		destfn = destfn[:len(destfn)-4]
	}
	ifh, ok := r.(*os.File)
	if !ok {
		var err error
		inpfn := destfn + "." + imgtyp
		ifh, err = os.Create(inpfn)
		if err != nil {
			return errors.Wrapf(err, "create temp image file "+inpfn)
		}
		if _, err = io.Copy(ifh, r); err != nil {
			Log("msg", "ImageToPdf reading", "file", ifh.Name(), "error", err)
		}
		if err = ifh.Close(); err != nil {
			Log("msg", "ImageToPdf writing", "dest", ifh.Name(), "error", err)
		}
		if ifh, err = os.Open(inpfn); err != nil {
			return errors.Wrapf(err, "open inp "+inpfn)
		}
		defer func() { _ = ifh.Close() }()
		if !LeaveTempFiles {
			defer func() { _ = unlink(inpfn, "ImageToPdf") }()
		}
	}
	destfn = destfn + ".pdf"
	if _, err := ifh.Seek(0, 0); err != nil {
		return err
	}
	if !fileExists(ifh.Name()) {
		Log("msg", "Input file not exist!", "file", ifh.Name())
		return errors.New("input file " + ifh.Name() + " not exists")
	}
	w, err := os.Create(destfn)
	if err != nil {
		return err
	}
	if err = ImageToPdfGm(ctx, w, ifh, contentType); err != nil {
		Log("msg", "ImageToPdfGm", "error", err)
	}
	closeErr := w.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// OfficeToPdf converts other to PDF with LibreOffice
func OfficeToPdf(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	getLogger(ctx).Log("msg", "Converting into", "ct", contentType, "dest", destfn)
	if strings.HasSuffix(destfn, ".pdf") {
		destfn = destfn[:len(destfn)-4]
	}
	inpfn := destfn + ".raw"
	fh, err := os.Create(inpfn)
	if err != nil {
		return err
	}
	defer func() { _ = unlink(inpfn, "OtherToPdf") }()
	if _, err = io.Copy(fh, r); err != nil {
		return err
	}
	return lofficeConvert(ctx, filepath.Dir(destfn), inpfn)
}

// OtherToPdf is the default converter
var OtherToPdf = OfficeToPdf

// PdfToPdf "converts" PDF (application/pdf) to PDF (just copies)
func PdfToPdf(ctx context.Context, destfn string, r io.Reader, _ string) error {
	getLogger(ctx).Log("msg", `"Converting" pdf into`, "dest", destfn)
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
		err = errors.Wrapf(err, "parse Content-Type %s", contentType)
		return err
	}

	parts := multipart.NewReader(r, params["boundary"])
	_, e := parts.NextPart()
	for e == nil {
		_, e = parts.NextPart()
	}
	if e != nil && e != io.EOF {
		return e
	}
	return nil
}

// HTMLToPdf converts HTML (text/html) to PDF
func HTMLToPdf(ctx context.Context, destfn string, r io.Reader, contentType string) error {
	Log := log.With(getLogger(ctx), "func", "HTMLToPdf", "dest", destfn).Log
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

		b, err := ioutil.ReadFile(inpfn)
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
					}
				}
				buf.Reset()
				if err = html.Render(&buf, img); err != nil {
					Log("msg", "html.Render", "img", img, "error", err)
					continue
				}
				Log("old", string(line), "new", buf.String())
				i := pos[0] + copy(b[pos[0]:pos[1]], buf.Bytes())
				for i < pos[1] {
					b[i] = ' '
					i++
				}
			}

			if err = ioutil.WriteFile(inpfn, b, 0644); err != nil {
				return errors.Wrap(err, "overwrite inpfn")
			}
		}
	}
	if *ConfWkhtmltopdf != "" {
		err := wkhtmltopdf(ctx, destfn, inpfn)
		if err == nil {
			return nil
		}
		Log("msg", "wkhtmltopdf", "error", err)
	}

	dn := filepath.Dir(destfn)
	outfn := filepath.Join(dn, filepath.Base(nakeFilename(inpfn))+".pdf")
	if err := lofficeConvert(ctx, dn, inpfn); err != nil {
		return err
	}
	if outfn != destfn {
		return moveFile(outfn, destfn)
	}
	return nil
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
func lofficeConvert(ctx context.Context, outDir, inpfn string) error {
	if outDir == "" {
		return errors.New("outDir is required!")
	}
	Log := getLogger(ctx).Log
	args := []string{"--headless", "--convert-to", "pdf", "--outdir",
		outDir, inpfn}
	lofficeMu.Lock()
	defer lofficeMu.Unlock()
	if lofficePortLock != nil {
		lofficePortLock.Lock()
		defer lofficePortLock.Unlock()
	}
	subCtx, subCancel := context.WithTimeout(ctx, *ConfLofficeTimeout)
	cmd := exec.CommandContext(subCtx, *ConfLoffice, args...)
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
		Log("msg", "env LC_ALL="+lcAll)
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
		return errors.Wrapf(err, "%q", cmd.Args)
	}
	outfn := filepath.Join(outDir, filepath.Base(nakeFilename(inpfn))+".pdf")
	if _, err := os.Stat(outfn); err != nil {
		return errors.Wrapf(err, "loffice no output for %s", filepath.Base(inpfn))
	}
	return nil
}

// calls wkhtmltopdf
func wkhtmltopdf(ctx context.Context, outfn, inpfn string) error {
	Log := getLogger(ctx).Log
	args := []string{
		inpfn,
		"--allow", "images",
		"--encoding", "utf-8",
		"--load-error-handling", "ignore",
		"--load-media-error-handling", "ignore",
		"--images",
		"--enable-local-file-access",
		"--no-background",
		outfn}
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, *ConfWkhtmltopdf, args...)
	cmd.Dir = filepath.Dir(inpfn)
	cmd.Stderr = &buf
	cmd.Stdout = os.Stdout
	Log("start", "wkhtmltopdf", "args", cmd.Args)
	if err := cmd.Run(); err != nil {
		err = errors.Wrapf(err, "%q", cmd.Args)
		if bytes.HasSuffix(buf.Bytes(), []byte("ContentNotFoundError\n")) ||
			bytes.HasSuffix(buf.Bytes(), []byte("ProtocolUnknownError\n")) ||
			bytes.HasSuffix(buf.Bytes(), []byte("HostNotFoundError\n")) { // K-MT11422:99503
			Log("msg", buf.String())
		} else {
			return errors.Wrapf(err, buf.String())
		}
	}
	if fi, err := os.Stat(outfn); err != nil {
		return errors.Wrapf(err, "wkhtmltopdf no output for %s", filepath.Base(inpfn))
	} else if fi.Size() == 0 {
		return errors.New("wkhtmltopdf empty output for " + filepath.Base(inpfn))
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
	"msg": "application/x-ole-storage",

	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"gif":  "image/gif",
	"png":  "image/png",
	"tif":  "image/tif",
	"tiff": "image/tiff",
}

func fixCT(contentType, fileName string) (ct string) {
	//defer func() {
	//	Log("msg", "fixCT", "ct", contentType, "fn", fileName, "result", ct)
	//}()

	switch contentType {
	case applicationZIP, "application/x-zip-compressed":
		if ext := filepath.Ext(fileName); len(ext) > 3 {
			// http://www.iana.org/assignments/media-types/media-types.xhtml#application
			switch ext {
			case ".docx", ".xlsx", ".pptx":
				return ExtContentType[ext[1:]]
			}
		}
		return applicationZIP
	case "application/x-rar-compressed", "application/x-rar":
		return "application/rar"
	case "image/pdf":
		return applicationPDF
	}
	return contentType
}

// FixContentType ensures proper content-type
// (uses filetype.v1 for "" and application/octet-stream)
func FixContentType(body []byte, contentType, fileName string) (ct string) {
	defer func() {
		if contentType != ct {
			Log("msg", "FixContentType", "ct", contentType, "fn", fileName, "result", ct)
		}
	}()

	contentType = fixCT(contentType, fileName)
	if ext := strings.ToLower(filepath.Ext(fileName)); strings.HasPrefix(ext, ".") {
		if want, ok := ExtContentType[ext[1:]]; ok && contentType != want {
			if !filetype.IsType(body, types.Type{MIME: types.NewMIME(contentType)}) {
				if typ, err := filetype.Match(body); err == nil && typ.MIME.Type != "" {
					return fixCT(typ.MIME.Type+"/"+typ.MIME.Subtype, fileName)
				}
			}
		}
	}
	c := GetConverter(contentType, nil)
	if c == nil { // no converter for this
		if filetype.IsType(body, types.Type{MIME: types.NewMIME(contentType)}) {
			if typ, err := filetype.Match(body); err == nil && typ.MIME.Type != "" {
				return fixCT(typ.MIME.Type+"/"+typ.MIME.Subtype, fileName)
			}
		}
	}
	if fileName != "" &&
		(contentType == "" || contentType == "application/octet-stream" || c == nil) {
		if ext := filepath.Ext(fileName); len(ext) > 3 {
			if nct, ok := ExtContentType[ext[1:]]; ok {
				return fixCT(nct, fileName)
			}
			if nct := mime.TypeByExtension(ext); nct != "" {
				return fixCT(nct, fileName)
			}
		}
	}
	//log.Printf("ct=%s ==> %s", ct, contentType)
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
	case "multipart/related":
		converter = MPRelatedToPdf
	case "application/x-pkcs7-signature":
		converter = Skip
	default:
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
