// +build ignore

// Copyright 2013 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/go-kit/kit/log"
)

func main() {
	Log := log.NewLogfmtLogger(os.Stderr).Log

	Log("msg", "go build")
	if err := run("go", "build"); err != nil {
		Log("msg", "go build", "error", err)
		os.Exit(1)
	}

	tmpdir, err := ioutil.TempDir("", "agostle-docker-")
	if err != nil {
		Log("msg", "create temp dir", "error", err)
		os.Exit(2)
	}
	dockfn := filepath.Join("docks", "Dockerfile")
	if err = copyFile(dockfn, filepath.Join(tmpdir, "Dockerfile")); err != nil {
		Log("msg", "copy Dockerfile", "error", err)
		os.Exit(3)
	}
	if err = copyFile("agostle", filepath.Join(tmpdir, "agostle")); err != nil {
		Log("msg", "copy agostle", "error", err)
		os.Exit(4)
	}

	Log("msg", "docker build")
	if err = run("docker", "build", "-t", "tgulacsi/agostle", tmpdir); err != nil {
		Log("msg", "docker build", "error", err)
		os.Exit(5)
	}
	Log("msg", "Done.")
	fmt.Println("# You can run your agostle in this shiny new docker:")
	fmt.Println("docker run -t -i -p 8500:8500 tgulacsi/agostle")
}

func run(executable string, args ...string) error {
	cmd := exec.Command(executable, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func copyFile(src, dst string) error {
	sfh, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sfh.Close()
	dfh, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err = io.Copy(dfh, sfh); err != nil {
		dfh.Close()
		return err
	}
	return dfh.Close()
}
