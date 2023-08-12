// Copyright 2017, 2023 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

// Package converter implements function for converting files to PDF
package converter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/UNO-SOFT/filecache"
	"github.com/UNO-SOFT/zlog/v2"
	"github.com/UNO-SOFT/zlog/v2/slog"
	config "github.com/stvp/go-toml-config"
	"github.com/tgulacsi/go/osgroup"
)

var logger = slog.Default()

func SetLogger(lgr *slog.Logger) { logger = lgr }

func lookPath(fn string) string {
	path, err := exec.LookPath(fn)
	if err != nil {
		return ""
	}
	return path
}

var (
	// ConfPdftk is the path for PdfTk
	ConfPdftk = config.String("pdftk", lookPath("pdftk"))

	// ConfPdfseparate is the path for pdfseparate (member of poppler-utils
	ConfPdfseparate = config.String("pdfseparate", "pdfseparate")

	// ConfLoffice is the path for LibreOffice
	ConfLoffice = config.String("loffice", lookPath("loffice"))

	// ConfGm is the path for GraphicsMagick
	ConfGm = config.String("gm", lookPath("gm"))

	// ConfGs is the path for GhostScript
	ConfGs = config.String("gs", lookPath("gs"))

	// ConfPdfClean is the path for pdfclean
	ConfPdfClean = config.String("pdfclean", lookPath("pdfclean"))

	// ConfMutool is the path for mutool
	ConfMutool = config.String("mutool", lookPath("mutool"))

	// ConvWkhtmltopdf is the parth for wkhtmltopdf
	ConfWkhtmltopdf = config.String("wkhtmltopdf", lookPath("wkhtmltopdf"))

	// ConfSortBeforeMerge should be true if generally we should sort files by filename before merge
	ConfSortBeforeMerge = config.Bool("sortBeforeMerge", false)

	// ConfChildTimeout is the time before the child gets killed
	ConfChildTimeout = config.Duration("childTimeout", 10*time.Minute)

	// ConfLofficeTimeout is the time before LibreOffice gets killed.
	ConfLofficeTimeout = config.Duration("lofficeTimeout", time.Minute)

	// ConcLimit limits the concurrently running child processes
	ConcLimit = NewRateLimiter(Concurrency)

	// ConfWorkdir is the working directory (will be os.TempDir() if empty)
	ConfWorkdir = config.String("workdir", "")

	// ConfListenAddr is a listen address for HTTP requests
	ConfListenAddr = config.String("listen", ":9500")

	// ConfDefaultIsService decides whether start as service without args
	ConfDefaultIsService = config.Bool("defaultIsService", false)

	// ConfUseLofficePortLock defines whether to limit Loffice usage by a port lock
	ConfLofficeUsePortLock = config.Bool("lofficeUsePortLock", !osgroup.IsInsideDocker())

	// ConfLogFile specifies the file to log - instead of command line.
	ConfLogFile = config.String("logfile", "")

	// ConfKeepRemoteImage specifiec whether to keep the remote sources of images (mg src="http://mailtrack...").
	ConfKeepRemoteImage = config.Bool("keepRemoteImage", false)

	// ConfGotenbertURL is the working Gotenbert (https://pkg.go.dev/github.com/gotenberg/gotenberg/v7) service URL
	ConfGotenbergURL = &gotenberg.URL
)

func init() {
	config.StringVar(ConfGotenbergURL, "gotenberg", "")
}

var gotenberg Gotenberg

// LoadConfig loads TOML config file
func LoadConfig(ctx context.Context, fn string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := config.Parse(fn); err != nil {
		logger.Info("WARN Cannot open config file", "file", fn, "error", err)
	}
	if *ConfLoffice != "" {
		if _, err := exec.LookPath(*ConfLoffice); err != nil {
			logger.Info("WARN cannot use as loffice!", "loffice", *ConfLoffice)
			if fn, err := exec.LookPath("soffice"); err == nil {
				logger.Info("Will use as loffice instead.", "soffice", fn)
				*ConfLoffice = fn
			}
		}
	}
	if *ConfWorkdir != "" {
		_ = os.Setenv("TMPDIR", *ConfWorkdir)
		Workdir = *ConfWorkdir
	}
	var err error
	cd := filepath.Join(Workdir, "agostle-filecache")
	// nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission
	_ = os.MkdirAll(cd, 0700)
	if Cache, err = filecache.Open(cd); err != nil {
		var tErr error
		if cd, tErr = os.MkdirTemp(Workdir, "agostle-filecache-*"); tErr != nil {
			return err
		} else if Cache, tErr = filecache.Open(cd); tErr != nil {
			return err
		}
	}

	bn := filepath.Base(*ConfPdfseparate)
	prefix := (*ConfPdfseparate)[:len(*ConfPdfseparate)-len(bn)]
	for k := range popplerOk {
		// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		if err := exec.CommandContext(ctx, prefix+k, "-h").Run(); err == nil {
			popplerOk[k] = prefix + k
		}
	}
	logger.Debug("LoadConfig", "popplerOk", popplerOk)

	if !*ConfLofficeUsePortLock {
		lofficeMu.Lock()
		lofficePortLock = nil
		lofficeMu.Unlock()
	}

	return nil
}

// Workdir is the main working directory
var Workdir = os.TempDir()
var Cache *filecache.Cache

// LeaveTempFiles should be true only for debugging purposes (leaves temp files)
var LeaveTempFiles = false

type ctxKeyWorkDir struct{}

func prepareContext(ctx context.Context, subdir string) (context.Context, string) {
	var wdKey ctxKeyWorkDir
	odir, _ := ctx.Value(wdKey).(string)
	if odir != "" {
		if subdir != "" {
			ctx = context.WithValue(ctx, wdKey, filepath.Join(Workdir, subdir))
		}
	} else {
		if subdir != "" {
			ctx = context.WithValue(ctx, wdKey, Workdir)
		} else {
			ctx = context.WithValue(ctx, wdKey, filepath.Join(Workdir, subdir))
		}
	}
	ndir, ok := ctx.Value(wdKey).(string)
	if ok && odir != ndir {
		// nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission
		if err := os.MkdirAll(ndir, 0750); err != nil {
			panic("cannot create workdir " + ndir + ": " + err.Error())
		}
	}
	return ctx, ndir
}

// port for LibreOffice locking (only one instance should be running)
const LofficeLockPort = 27999

// save original html (do not delete it)
var SaveOriginalHTML = false

// name of errors list in resulting archive
const ErrTextFn = "ZZZ-errors.txt"

func getLogger(ctx context.Context) *slog.Logger {
	if lgr := zlog.SFromContext(ctx); lgr != nil {
		return lgr
	}
	return logger
}
