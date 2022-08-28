//go:build ignore
// +build ignore

// Copyright 2017, 2022 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tgulacsi/go/zlog"
)

func main() {
	logger = zlog.New(zlog.MaybeConsoleWriter(os.Stderr))

	logger.Info("go build")
	if err := run("go", "build"); err != nil {
		logger.Info("go build", "error", err)
		os.Exit(1)
	}

	tmpdir, err := os.MkdirTemp("", "agostle-docker-")
	if err != nil {
		logger.Info("create temp dir", "error", err)
		os.Exit(2)
	}
	dockfn := filepath.Join("docks", "Dockerfile")
	if err = copyFile(dockfn, filepath.Join(tmpdir, "Dockerfile")); err != nil {
		logger.Info("copy Dockerfile", "error", err)
		os.Exit(3)
	}
	if err = copyFile("agostle", filepath.Join(tmpdir, "agostle")); err != nil {
		logger.Info("copy agostle", "error", err)
		os.Exit(4)
	}

	logger.Info("docker build")
	if err = run("docker", "build", "-t", "tgulacsi/agostle", tmpdir); err != nil {
		logger.Info("docker build", "error", err)
		os.Exit(5)
	}
	logger.Info("Done.")
	fmt.Println("# You can run your agostle in this shiny new docker:")
	fmt.Println("docker run -t -i -p 8500:8500 tgulacsi/agostle")
}

func run(ctx context.Context, executable string, args ...string) error {
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := exec.CommandContext(ctx, executable, args...)
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
	dfh, err := renameio.NewPendingFile(dst)
	if err != nil {
		return err
	}
	defer dfh.Cleanup()
	if _, err = io.Copy(dfh, sfh); err != nil {
		dfh.Close()
		return err
	}
	return dfh.CloseAtomicallyRename()
}
