// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/tgulacsi/go/temp"
)

const png = "png"

func command(ctx context.Context, prg string, args ...string) *exec.Cmd {
	if prg == "" {
		prg, args = args[0], args[1:]
	}
	return exec.CommandContext(ctx, prg, args...)
}

// ImageToPdfGm converts image to PDF using GraphicsMagick
func ImageToPdfGm(ctx context.Context, w io.Writer, r io.Reader, contentType string) error {
	//log.Printf("converting image %s to %s", contentType, destfn)
	imgtyp := ""
	if false && contentType != "" {
		imgtyp = contentType[strings.Index(contentType, "/")+1:] + ":"
	}

	cmd := command(ctx, *ConfGm, "convert", imgtyp+"-", "pdf:-")
	// cmd.Stdin = io.TeeReader(r, os.Stderr)
	cmd.Stdin = r
	cmd.Stdout = w
	errout := bytes.NewBuffer(nil)
	cmd.Stderr = errout
	if err := cmd.Run(); err != nil {
		err = fmt.Errorf("%q: %w", cmd.Args, err)
		return fmt.Errorf("gm convert converting %s: %s: %w", r, errout.Bytes(), err)
	}
	if len(errout.Bytes()) > 0 {
		logger.Info("WARN gm convert", "r", r, "error", errout.String())
	}
	return nil
}

// PdfToImage converts PDF to image using PdfToImageGm if available and the result is OK, then PdfToImageCairo.
func PdfToImage(ctx context.Context, w io.Writer, r io.Reader, contentType, size string) error {
	src := temp.NewMemorySlurper("PdfToImage-src-")
	defer src.Close()
	dst := temp.NewMemorySlurper("PdfToImage-dst-")
	defer dst.Close()

	var err error
	if err = PdfToImageCairo(ctx, dst, io.TeeReader(r, src), contentType, size); err == nil {
		_, err = io.Copy(w, dst)
		return err
	}
	logger.Info("ERROR PdfToImageCairo", "error", err)
	return PdfToImageGm(ctx, w, io.MultiReader(src, r), contentType, size)
}

// PdfToImageCairo converts PDF to image using pdftocairo from poppler-utils.
func PdfToImageCairo(ctx context.Context, w io.Writer, r io.Reader, contentType, size string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	imgtyp, ext := "gif", png
	if contentType != "" && strings.HasPrefix(contentType, "image/") {
		imgtyp = contentType[6:]
	}
	if imgtyp == png || imgtyp == "jpeg" {
		ext = imgtyp
	}
	args := append(make([]string, 0, 8), "-singlefile", "-"+ext, "-cropbox")
	if size != "" {
		i := strings.IndexByte(size, 'x')
		if i <= 0 || size[:i] == size[i+1:] {
			if i > 0 {
				size = size[:i]
			}
			args = append(args, "-scale-to", size)
		} else {
			args = append(args, "-scale-to-x", size[:i])
			args = append(args, "-scale-to-y", size[i+1:])
		}
	}
	tfh, err := os.CreateTemp("", "PdfToImageGm-")
	if err != nil {
		logger.Info("ERROR cannot create temp file", "error", err)
		return err
	}
	tfh.Close()
	_ = os.Remove(tfh.Name())
	fn := tfh.Name()
	args = append(args, "-", fn)
	fn = fn + "." + ext // pdftocairo appends the .png

	cmd := exec.CommandContext(ctx, "pdftocairo", args...)
	cmd.Stdin = r
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("%q: %w", cmd.Args, err)
	}
	if tfh, err = os.Open(fn); err != nil {
		logger.Info("ERROR cannot open temp", "file", fn, "error", err)
		return err
	}
	defer tfh.Close()
	_ = os.Remove(fn)

	if imgtyp == png {
		_, err = io.Copy(w, tfh)
		return err
	}

	// convert to the requested format
	cmd = command(ctx, *ConfGm, "convert", "png:-", imgtyp+":-")
	cmd.Stdin = tfh
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// PdfToImageGm converts PDF to image using GraphicsMagick.
func PdfToImageGm(ctx context.Context, w io.Writer, r io.Reader, contentType, size string) error {
	// gm may pollute its stdout with error & warning messages, so we must use files!
	var imgtyp = "gif"
	if contentType != "" && strings.HasPrefix(contentType, "image/") {
		imgtyp = contentType[6:]
	}
	args := make([]string, 3, 5)
	args[0], args[1] = "convert", "pdf:-"
	if size != "" {
		args[2] = "-resize"
		args = append(args, size)
	}
	tfh, err := os.CreateTemp("", "PdfToImageGm-")
	if err != nil {
		logger.Info("ERROR cannot create temp file", "error", err)
		return err
	}
	args = append(args, imgtyp+":"+tfh.Name()) // this MUST be : (colon)!
	cmd := command(ctx, *ConfGm, args...)
	cmd.Stdin = r
	//cmd.Stdout = &filterFirstLines{Beginning: []string{"Can't find ", "Warning: "}, Writer: w}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("%q: %w", cmd.Args, err)
	}
	fn := tfh.Name()
	tfh.Close()
	if tfh, err = os.Open(fn); err != nil {
		logger.Info("ERROR cannot open temp file", "file", fn, "error", err)
		return err
	}
	_ = os.Remove(fn)
	_, err = io.Copy(w, tfh)
	_ = tfh.Close()
	_ = os.Remove(fn)
	return err
}
