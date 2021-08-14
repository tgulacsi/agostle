// Copyright 2019, 2020 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tgulacsi/go/temp"
)

func fileExists(fn string) bool {
	if _, err := os.Stat(fn); err == nil {
		return true
	}
	return false
}

// move file
func moveFile(from, to string) error {
	if from == to {
		return nil
	}
	if os.Rename(from, to) == nil {
		return nil
	}
	if err := copyFile(from, to); err != nil {
		return err
	}
	_ = os.Remove(from) // ignore error
	return nil
}

// copy file
func copyFile(from, to string) error {
	if from == to {
		return nil
	}
	ifh, err := os.Open(from)
	if err != nil {
		return fmt.Errorf("copy cannot open %s for reading: %w", from, err)
	}
	defer func() { _ = ifh.Close() }()
	ofh, err := os.Create(to)
	if err != nil {
		return fmt.Errorf("copy cannot open %s for writing: %w", to, err)
	}
	if _, err = io.Copy(ofh, ifh); err != nil {
		return fmt.Errorf("error copying from %s to %s: %w", from, to, err)
	}
	return nil
}

func openOut(destfn string) (*os.File, error) {
	if destfn == "" || destfn == "-" {
		return os.Stdout, nil
	}
	return os.Create(destfn)
}

// return filename with extension stripped
func nakeFilename(fn string) string {
	if ext := filepath.Ext(fn); ext != "" {
		return fn[:len(fn)-len(ext)]
	}
	return fn
}

type Statter interface {
	Stat() (os.FileInfo, error)
}

// FileLike is a minimal needed interface for ArchFileItem.File
type FileLike interface {
	io.Reader
	io.Closer
	Statter
}

func MakeFileLike(r io.Reader) FileLike {
	switch r := r.(type) {
	case FileLike:
		return r
	case *io.SectionReader:
		return ReadCloserFileLike{
			Reader: r,
			Closer: ioutil.NopCloser(nil),
			FileInfo: dummyFileInfo{
				name: fmt.Sprintf("file-like-%p", r),
				size: r.Size(),
				time: time.Now(),
			},
		}
	case *bytes.Buffer:
		return ReadCloserFileLike{
			Reader: bytes.NewReader(r.Bytes()),
			Closer: ioutil.NopCloser(nil),
			FileInfo: dummyFileInfo{
				name: fmt.Sprintf("file-like-%p", r),
				size: int64(r.Len()),
				time: time.Now(),
			},
		}
	}
	c, ok := r.(io.Closer)
	if !ok {
		c = ioutil.NopCloser(nil)
	}
	rc := ReadCloserFileLike{
		Reader: r,
		Closer: c,
	}
	//Log("msg", "MakeFileLike", "r", fmt.Sprintf("%T %#v", r, r))
	if s, ok := r.(Statter); ok {
		rc.FileInfo, rc.statErr = s.Stat()
	} else {
		rs, err := temp.NewReadSeeker(r)
		rc.statErr = err
		size, _ := rs.Seek(0, 2)
		rs.Seek(0, 0)
		rc.Reader, rc.Closer = rs, rs
		rc.FileInfo = dummyFileInfo{
			name: fmt.Sprintf("file-like-%p", r),
			size: size,
			time: time.Now(),
		}
	}
	return rc
}

type ReadCloserFileLike struct {
	io.Reader
	io.Closer
	os.FileInfo
	statErr error
}

func (fl ReadCloserFileLike) Stat() (os.FileInfo, error) {
	return fl.FileInfo, fl.statErr
}

// ArchFileItem groups an archive item
type ArchFileItem struct {
	File     FileLike //opened file handle
	Error    error    //error
	Filename string   //name of the file
	Archive  string   //name in the archive
}

// ArchiveName returns the archive name - Archive, Filename if set, otherwise File's name
func (a ArchFileItem) ArchiveName() string {
	if a.Archive != "" {
		return a.Archive
	}
	if a.Filename != "" {
		return a.Filename
	}
	if a.File != nil {
		fi, err := a.File.Stat()
		if err != nil {
			return fi.Name()
		}
	}
	return ""
}

// ArchItems is a wrapper for []ArchFileItem for sort.Sort
type ArchItems []ArchFileItem

// Len returns the length of ArchItems
func (a ArchItems) Len() int { return len(a) }

// Less returns whether a[i] < a[j]
func (a ArchItems) Less(i, j int) bool { return a[i].ArchiveName() < a[j].ArchiveName() }

// Swap swaps items i and j for sort.Sort
func (a ArchItems) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

// Sort sorts ArchItems ArchiveName-ordered
func (a ArchItems) Sort() ArchItems { sort.Sort(a); return a }

type syncer interface {
	Sync() error
}

// ZipFiles adds files (by handle) to zip (writer)
func ZipFiles(dest io.Writer, skipOnError, unsafeArchFn bool, files ...ArchFileItem) (err error) {
	filesch := make(chan ArchFileItem)
	go func() {
		for _, item := range files {
			filesch <- item
		}
		close(filesch)
	}()
	return zipFiles(dest, skipOnError, unsafeArchFn, filesch)
}

// ZipTree adds all files in the tree originating the given path to zip (writer)
func ZipTree(dest io.Writer, root string, skipOnError, unsafeArchFn bool) (err error) {
	filesch := make(chan ArchFileItem)
	root = filepath.Clean(root) + string([]rune{filepath.Separator})
	rootLen := len(root)
	go func() {
		_ = filepath.Walk(root,
			func(path string, info os.FileInfo, err error) error {
				if !info.IsDir() && info.Mode()&os.ModeType == 0 {
					filesch <- ArchFileItem{Filename: path, Archive: path[rootLen:]}
				}
				return nil
			})
		close(filesch)
	}()
	return zipFiles(dest, skipOnError, unsafeArchFn, filesch)
}

// zipFiles adds files transferred through the channel to zip (writer)
func zipFiles(dest io.Writer, skipOnError, unsafeArchFn bool, files <-chan ArchFileItem) (err error) {
	if dfh, ok := dest.(syncer); ok {
		defer func() {
			if e := dfh.Sync(); e != nil && err == nil && !strings.Contains(e.Error(), "invalid argument") {
				err = e
			}
		}()
	}
	zfh := zip.NewWriter(dest)
	defer func() {
		if e := zfh.Close(); e != nil && err == nil {
			err = e
		}
	}()
	var (
		fi         os.FileInfo
		zi         *zip.FileHeader
		w          io.Writer
		errs       []error
		openedHere bool
	)
	appendErr := func(err error) {
		if err == nil || err.Error() == "" {
			return
		}
		if errs == nil {
			errs = []error{err}
		} else {
			errs = append(errs, err)
		}
	}

	for item := range files {
		openedHere = false
		if item.File == nil {
			if item.Filename == "" {
				continue
			}
			openedHere = true
			if item.File, err = os.Open(item.Filename); err != nil {
				err = fmt.Errorf("zip cannot open %q: %w", item.Filename, err)
				if !skipOnError {
					return err
				}
				appendErr(err)
				continue
			}
		}
		if fi, err = item.File.Stat(); err != nil {
			if openedHere {
				_ = item.File.Close()
			}
			err = fmt.Errorf("error stating %s: %w", item.File, err)
		} else if fi == nil {
			err = fmt.Errorf("nil stat of %#v", item.File)
		}
		if err != nil {
			if !skipOnError {
				return err
			}
			appendErr(err)
			continue
		}
		if zi, err = zip.FileInfoHeader(fi); err != nil {
			if openedHere {
				_ = item.File.Close()
			}
			err = fmt.Errorf("convert stat %s to header: %w", item.File, err)
			if !skipOnError {
				return err
			}
			appendErr(err)
			continue
		}
		if item.Archive != "" {
			zi.Name = item.Archive
		} else if unsafeArchFn {
			zi.Name = unsafeFn(zi.Name, true)
		}
		if w, err = zfh.CreateHeader(zi); err != nil {
			if openedHere {
				_ = item.File.Close()
			}
			err = fmt.Errorf("creating header for %q: %w", zi.Name, err)
			if !skipOnError {
				return err
			}
			appendErr(err)
			continue
		}
		_, err = io.Copy(w, item.File)
		if openedHere {
			_ = item.File.Close()
		}
		if err != nil {
			err = fmt.Errorf("writing %s to zipfile: %w", item.File, err)
			Log("msg", "ERROR write to zip", "error", err)
			if !skipOnError {
				return err
			}
			appendErr(err)
			continue
		}
	}
	Log("msg", "zipFiles", "errors", errs)
	if len(errs) == 0 {
		return nil
	}
	sarr := make([]string, 0, len(errs))
	for _, err = range errs {
		sarr = append(sarr, err.Error())
	}
	return errors.New(strings.Join(sarr, "\n"))
}

func safeFn(fn string, maskPercent bool) string {
	fn = url.QueryEscape(
		strings.Replace(strings.Replace(fn, "/", "-", -1),
			`\`, "-", -1))
	if maskPercent {
		fn = strings.Replace(fn, "%", "!P!", -1)
	}
	return fn
}
func unsafeFn(fn string, maskPercent bool) string {
	if fn == "" {
		return fn
	}
	res := fn
	if maskPercent {
		res = strings.Replace(fn, "!P!", "%", -1)
		if res == "" {
			Log("msg", "WARN unsafeFn empty string from "+fn)
			res = fn
		}
	}
	fn = res
	var err error
	if res, err = url.QueryUnescape(fn); err != nil {
		Log("msg", "WARN cannot url.QueryUnescape", "fn", fn, "error", err)
		res = fn
	}
	return res
}

func unlink(fn, mark string) error {
	_ = mark
	return os.Remove(fn)
}

func unlinkAll(path string) error {
	return os.RemoveAll(path)
}

func fileContentHash(fn string) (hash.Hash, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	hsh := sha1.New()
	_, err = io.Copy(hsh, f)
	f.Close()
	return hsh, err
}

func headerGetFileName(hdr map[string][]string) string {
	for _, fn := range hdr["X-Filename"] {
		if fn != "" {
			return fn
		}
	}
	for _, mt := range hdr["Content-Disposition"] {
		_, params, err := mime.ParseMediaType(mt)
		if err == nil && params["filename"] != "" {
			return params["filename"]
		}
	}
	for _, desc := range hdr["Content-Description"] {
		if desc != "" {
			return desc
		}
	}
	return ""
}

var _ = os.FileInfo(dummyFileInfo{})

type dummyFileInfo struct {
	time time.Time
	name string
	size int64
	mode os.FileMode
}

func (fi dummyFileInfo) Name() string       { return fi.name }
func (fi dummyFileInfo) Size() int64        { return fi.size }
func (fi dummyFileInfo) Mode() os.FileMode  { return fi.mode }
func (fi dummyFileInfo) ModTime() time.Time { return fi.time }
func (fi dummyFileInfo) IsDir() bool        { return false }
func (fi dummyFileInfo) Sys() interface{}   { return nil }
