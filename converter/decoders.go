// Copyright 2019 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"path/filepath"

	"github.com/tgulacsi/go/i18nmail"
)

type htmlEscaper struct {
	w io.Writer
}

func (e htmlEscaper) Write(p []byte) (int, error) {
	_, err := io.WriteString(e.w, html.EscapeString(string(p)))
	return len(p), err
}

func regulateCid(text []byte, subDir string) []byte {
	nfn := bytes.Trim(text, "<>")
	//if k := bytes.IndexByte(nfn, '@'); k >= 0 {
	//nfn = nfn[:k]
	//}
	return []byte(subDir + "/" + filepath.Base(string(nfn)))
}

// ScanLines is a split function for a Scanner that returns each line of
// text, unmodified. The returned line may be empty.
// The end-of-line marker is one optional carriage return followed
// by one mandatory newline. In regular expression notation, it is `\r?\n`.
// The last non-empty line of input will be returned even if it has no
// newline.
func ScanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		// We have a full newline-terminated line.
		return i + 1, data[0 : i+1], nil
	}
	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), data, nil
	}
	// Request more data.
	return 0, nil, nil
}

// NewCidMapper remaps Content-Id urls to ContentDir/filename and returns the map
func NewCidMapper(cids map[string]string, subDir string, r io.Reader) io.Reader {
	data, _ := ioutil.ReadAll(r)
	start := []byte(`src="cid:`)
	var offset int
	result := make([]byte, 0, 2*len(data))
	for {
		i := bytes.Index(data[offset:], start)
		if i < 0 {
			break
		}
		i += offset
		j := bytes.IndexByte(data[i+len(start):], '"')
		if j < 0 {
			break
		}
		j += i + len(start)
		i += 5
		result = append(result, data[offset:i]...)
		offset = j

		key := data[i+4 : j]
		nfn := regulateCid(key, subDir)
		result = append(result, nfn...)
		cids[string(key)] = string(nfn)
	}
	return bytes.NewReader(append(result, data[offset:]...))
}

// NewEqsignStripper returns a reader which strips equal signs from line endings
func NewEqsignStripper(r io.Reader) io.Reader {
	split := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		//defer func() {
		//    log.Printf("split(%d, %b) => (%d, %d, %s)", len(data), atEOF,
		//        advance, len(token), err)
		//    }()
		if atEOF && len(data) == 0 {
			return 0, nil, io.EOF
		}
		i := bytes.IndexByte(data, '=')
		if i < 0 {
			return len(data), data, nil
		}
		if len(data) < i+1 {
			if i == 0 {
				if atEOF {
					return len(data), nil, nil
				}
				return 0, nil, nil
			}
			return i, data[:i], nil
		}
		if data[i+1] == '\n' {
			return i + 2, data[:i], nil
		}
		if data[i+1] == '\r' {
			if len(data) <= i+2 {
				if atEOF {
					return len(data), data, nil
				}
				return i, data[:i], nil
			}
			if data[i+2] == '\n' {
				return i + 3, data[:i], nil
			}
		}
		if atEOF {
			return len(data), data, nil
		}
		// not =\r?\n
		return i + 1, data[:i+1], nil
	}
	s := bufio.NewScanner(r)
	s.Split(split)
	return NewScannerReader(s)
}
func fromHex(b byte) (byte, error) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', nil
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, nil
	}
	return 0, fmt.Errorf("decoders: invalid quoted-printable hex byte 0x%02x", b)
}

func readHexByte(v []byte) (b byte, err error) {
	if len(v) < 2 {
		return 0, io.ErrUnexpectedEOF
	}
	var hb, lb byte
	if hb, err = fromHex(v[0]); err != nil {
		return 0, err
	}
	if lb, err = fromHex(v[1]); err != nil {
		return 0, err
	}
	return hb<<4 | lb, nil
}

// NewQuoPriDecoder replaces =A0= with \n
func NewQuoPriDecoder(r io.Reader) io.Reader {
	br := bufio.NewReader(r)
	if first, err := br.Peek(br.Buffered()); err != nil || !bytes.Contains(first, []byte("=C3")) {
		return br
	}
	r = NewEqsignStripper(br)
	var b byte
	split := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, io.EOF
		}
		i := bytes.IndexByte(data, '=')
		if i < 0 {
			return len(data), data, nil
		}
		if i+3 >= len(data) {
			return i, data[:i], nil
		}
		if b, err = readHexByte(data[i+1 : i+3]); err != nil {
			Log("msg", "readHexByte", "i", i, "error", err)
			return i + 3, data[:i+3], nil
		}
		return i + 3, append(data[:i], b), nil
	}
	s := bufio.NewScanner(r)
	s.Split(split)
	return NewScannerReader(s)
}

// NewB64QuoPriDecoder replaces bork encoding (+base64-)
func NewB64QuoPriDecoder(r io.Reader) io.Reader {
	r = NewEqsignStripper(r)
	buf := make([]byte, 4)
	split := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		//defer func() {
		//	log.Printf("B64-split(%s, %b) => (%d, %s, %s)", data, atEOF,
		//		advance, token, err)
		//}()
		if atEOF && len(data) == 0 {
			return 0, nil, io.EOF
		}
		i := bytes.IndexByte(data, '+')
		if i < 0 {
			return len(data), data, nil
		}
		j := bytes.IndexByte(data[i+1:], '-')
		if j < 0 {
			if atEOF {
				return len(data), data, nil
			}
			if i == 0 { //nil must be the token!
				return 0, nil, nil
			}
			return i, data[:i], nil
		} else if j == 0 {
			return i + 1, data[:i+1], nil
		}
		n := j
		j += i + 1

		b64r := i18nmail.NewB64Decoder(base64.StdEncoding, bytes.NewReader(data[i+1:j]))
		if n > len(buf) {
			if n <= cap(buf) {
				buf = buf[:n]
			} else {
				buf = append(buf, make([]byte, n-len(buf))...)
			}
		}
		//log.Printf("j=%d len(buf)=%d", j, len(buf))
		n, err = io.ReadFull(b64r, buf)
		//log.Printf("i=%d j=%d key=%s n=%d buf=%s err=%s", i, j, data[i+1:j],
		//	n, buf, err)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			Log("msg", "NewQuoPriDecode cannot decode", "msg", data[i:j+1])
			return j + 1, data[:j+1], nil
		}
		return j + 1, append(data[:i], buf[:n]...), nil
	}
	s := bufio.NewScanner(r)
	s.Split(split)
	return NewScannerReader(s)
}

// ScannerReader uses a bufio.Scanner as an io.Reader
type ScannerReader struct {
	s       *bufio.Scanner
	part    []byte
	stopped bool
}

// NewScannerReader turns a bufio.Scanner to an io.Reader
func NewScannerReader(s *bufio.Scanner) io.Reader {
	return &ScannerReader{s: s, part: make([]byte, 0, 4096), stopped: false}
}

// Implements io.Reader: reads at most len(p) bytes into p,
// returns the number of bytes read and/or the error encountered
func (sr *ScannerReader) Read(p []byte) (n int, err error) {
	//defer func() {
	//	log.Printf("Read(%d) => %d,%s [%s] part=%d stopped?%b", len(p), n,
	//		p[:n], err, len(sr.part), sr.stopped)
	//}()
	if sr.stopped {
		return 0, io.EOF
	}
	np := len(p)

	for len(sr.part) < np && !sr.stopped {
		if ok := sr.s.Scan(); !ok {
			sr.stopped = true
			if err = sr.s.Err(); err != nil {
				return 0, err
			}
			break
		}
		//log.Printf("read [%s]", sr.s.Bytes())
		if len(sr.s.Bytes()) > 0 {
			sr.part = append(sr.part, sr.s.Bytes()...)
		}
	}

	n = len(sr.part)
	//log.Printf("len(part)=%d len(p)=%d", n, np)
	if n > 0 {
		if n > np {
			n = np
			copy(p, sr.part[:n])
			sr.part = sr.part[n:]
			return
		}
		copy(p, sr.part)
		sr.part = sr.part[:0]
		return
	}
	err = io.EOF
	return
}
