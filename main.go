// Copyright 2017, 2022 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"context"

	"github.com/go-logr/zerologr"
	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/rs/zerolog"
	tufclient "github.com/theupdateframework/go-tuf/client"

	"github.com/kardianos/osext"
	"github.com/tgulacsi/agostle/converter"
	"github.com/tgulacsi/go/globalctx"
	"github.com/tgulacsi/go/i18nmail"
	"golang.org/x/sync/errgroup"
)

// go:generate sh -c "overseer-bindiff printkeys --go-out agostle-keyring.gpg >overseer_keyring.go"

const defaultUpdateURL = "https://www.unosoft.hu/tuf"

var zl = zerolog.New(os.Stderr).With().Timestamp().Logger().Level(zerolog.InfoLevel)
var logger = zerologr.New(&zl)

func main() {
	if err := Main(); err != nil {
		logger.Error(err, "Main")
		os.Exit(1)
	}
}

var (
	configFile, listenAddr string

	subcommands []*ffcli.Command
)

func newFlagSet(name string) *flag.FlagSet { return flag.NewFlagSet(name, flag.ContinueOnError) }

func Main() error {
	converter.SetLogger(logger.WithName("converter"))

	i18nmail.SetLogger(logger.V(1).WithName("i18nmail"))

	updateURL := defaultUpdateURL
	var (
		verbose, leaveTempFiles bool
		concurrency             int
		timeout                 time.Duration
		logFile                 string
	)

	fs := newFlagSet("agostle")
	fs.StringVar(&updateURL, "update-url", updateURL, "URL to download updates from (with GOOS and GOARCH template vars)")
	fs.BoolVar(&leaveTempFiles, "x", false, "leave tempfiles?")
	fs.BoolVar(&verbose, "v", false, "verbose logging")
	fs.IntVar(&concurrency, "concurrency", converter.Concurrency, "number of childs start in parallel")
	fs.DurationVar(&timeout, "timeout", 10*time.Minute, "timeout for external programs")
	fs.StringVar(&configFile, "config", "", "config file (TOML)")
	fs.StringVar(&logFile, "logfile", "", "logfile")
	appCmd := &ffcli.Command{
		Name:        "agostle",
		ShortHelp:   "agostle is an \"apostle\" which turns everything to PDF",
		FlagSet:     fs,
		Subcommands: subcommands,
	}

	var updateRootJSON, updateRootKeys string
	fs = newFlagSet("update")
	fs.StringVar(&updateRootKeys, "root-keys-string", defaultRootKeys, "CONTENTS of root.json for TUF update")
	fs.StringVar(&updateRootJSON, "root-keys-file", updateRootJSON, "PATH of root.json for TUF update")
	updateCmd := ffcli.Command{Name: "update", ShortHelp: "update binary to the latest release", FlagSet: fs,
		Exec: func(ctx context.Context, args []string) error {
			self, err := os.Executable()
			if err != nil {
				return err
			}
			logger.Info("update", "from", updateURL)
			remote, err := tufclient.HTTPRemoteStore(updateURL, nil, nil)
			if err != nil {
				return err
			}
			rootKeySrc := []byte(updateRootKeys)
			if updateRootJSON != "" {
				logger.Info("using root keys", "from", updateRootJSON)
				var readErr error
				if rootKeySrc, readErr = ioutil.ReadFile(updateRootJSON); readErr != nil {
					return readErr
				}
			}
			tc := tufclient.NewClient(tufclient.MemoryLocalStore(), remote)
			if err := tc.InitLocal(rootKeySrc); err != nil {
				return fmt.Errorf("init: %w", err)
			}
			targets, err := tc.Update()
			if err != nil {
				return fmt.Errorf("update: %w", err)
			}
			logger.Info("config", "targets", targets)

			destFh, err := os.Create(
				filepath.Join(filepath.Dir(self), "."+filepath.Base(self)+".new"),
			)
			if err != nil {
				return err
			}
			defer destFh.Close()
			logger.Info("download", "to", destFh.Name())
			dest := &downloadFile{File: destFh}
			if err := tc.Download(
				strings.Replace(strings.Replace(
					"/agostle/{{GOOS}}_{{GOARCH}}",
					"{{GOOS}}", runtime.GOOS, -1),
					"{{GOARCH}}", runtime.GOARCH, -1),
				dest,
			); err != nil {
				return fmt.Errorf("download: %w", err)
			}
			_ = os.Chmod(destFh.Name(), 0775)

			old := filepath.Join(filepath.Dir(self), "."+filepath.Base(self)+".old")
			logger.Info("rename", "from", self, "to", old)
			if err := os.Rename(self, old); err != nil {
				return err
			}
			logger.Info("rename", "from", destFh.Name(), "to", self)
			if err := os.Rename(destFh.Name(), self); err != nil {
				logger.Error(err, "rename", "from", destFh.Name(), "to", self)
			} else {
				os.Remove(old)
			}

			return nil
		},
	}
	appCmd.Subcommands = append(appCmd.Subcommands, &updateCmd)

	var savereq bool
	var regularUpdates time.Duration
	fs = newFlagSet("serve")
	fs.DurationVar(&regularUpdates, "regular-updates", 0, "do regular updates at this interval")
	fs.BoolVar(&savereq, "savereq", false, "save requests")
	serveCmd := ffcli.Command{Name: "serve", ShortHelp: "serve HTTP",
		ShortUsage: "agostle serve [flags] [addr.to.listen.on:port]", FlagSet: fs,
		Exec: func(ctx context.Context, args []string) error {
			if len(args) != 0 {
				listenAddr = args[0]
			}
			listeners := getListeners()
			if listenAddr == "" && len(listeners) == 0 {
				listenAddr = *converter.ConfListenAddr
			}
			logger.Info("serve", "listeners", len(listeners), "listenAddr", listenAddr)

			grp, grpCtx := errgroup.WithContext(ctx)
			srvs := make([]*http.Server, 0, len(listeners)+1)
			if listenAddr != "" {
				grp.Go(func() error {
					logger.Info("listening", "address", listenAddr)
					s := newHTTPServer(listenAddr, savereq)
					srvs = append(srvs, s)
					return s.ListenAndServe()
				})
			}
			for _, l := range listeners {
				l := l
				grp.Go(func() error {
					logger.Info("listening", "listener", l)
					s := newHTTPServer("", savereq)
					srvs = append(srvs, s)
					return s.Serve(l)
				})
			}
			<-grpCtx.Done()
			for _, l := range listeners {
				l.Close()
			}
			for _, s := range srvs {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_ = s.Shutdown(ctx)
				cancel()
				_ = s.Close()
			}
			return grp.Wait()
		},
	}
	appCmd.Subcommands = append(appCmd.Subcommands, &serveCmd)

	if err := appCmd.Parse(os.Args[1:]); err != nil {
		return err
	}

	var closeLogfile func() error

	var err error
	if closeLogfile, err = logToFile(logFile); err != nil {
		return err
	}
	if verbose {
		i18nmail.SetLogger(logger)
	} else {
		i18nmail.SetLogger(logger.V(1))
	}
	logger.Info("config", "leave_tempfiles?", leaveTempFiles)
	converter.LeaveTempFiles = leaveTempFiles
	converter.Concurrency = concurrency
	if configFile == "" {
		if self, execErr := osext.Executable(); execErr != nil {
			logger.Info("Cannot determine executable file name", "error", execErr)
		} else {
			ini := filepath.Join(filepath.Dir(self), "agostle.ini")
			f, iniErr := os.Open(ini)
			if iniErr != nil {
				logger.Info("Cannot open config", "file", ini, "error", iniErr)
			} else {
				_ = f.Close()
				configFile = ini
			}
		}
	}
	ctx, cancel := globalctx.Wrap(context.Background())
	defer cancel()
	logger.Info("Loading config", "file", configFile)
	if err = converter.LoadConfig(ctx, configFile); err != nil {
		logger.Info("Parsing config", "file", configFile, "error", err)
		return err
	}
	if timeout > 0 && timeout != *converter.ConfChildTimeout {
		logger.Info("Setting timeout", "from", *converter.ConfChildTimeout, "to", timeout)
		*converter.ConfChildTimeout = timeout
	}
	if closeLogfile == nil {
		if closeLogfile, err = logToFile(*converter.ConfLogFile); err != nil {
			logger.Error(err, "logToFile")
		}
	}

	sortBeforeMerge = *converter.ConfSortBeforeMerge
	logger.Info("commands",
		"pdftk", *converter.ConfPdftk,
		"loffice", *converter.ConfLoffice,
		"gm", *converter.ConfGm,
		"gs", *converter.ConfGs,
		"pdfclean", *converter.ConfPdfClean,
		"wkhtmltopdf", *converter.ConfWkhtmltopdf,
	)
	logger.Info("parameters",
		"sortBeforeMerge", sortBeforeMerge,
		"workdir", converter.Workdir,
		"listen", *converter.ConfListenAddr,
		"childTimeout", *converter.ConfChildTimeout,
		"defaultIsService", *converter.ConfDefaultIsService,
		"logfile", *converter.ConfLogFile,
	)

	updateURL = strings.NewReplacer("{{.GOOS}}", runtime.GOOS, "{{.GOARCH}}", runtime.GOARCH).Replace(updateURL)

	if closeLogfile != nil {
		defer func() {
			logger.Info("close log file", "error", closeLogfile())
		}()
	}

	return appCmd.Run(ctx)
}

func logToFile(fn string) (func() error, error) {
	if fn == "" {
		return nil, nil
	}
	fh, err := os.OpenFile(fn, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		logger.Error(err, "open log file", "file", fn)
		return nil, fmt.Errorf("%s: %w", fn, err)
	}
	logger.Info("Will log to", "file", fh.Name())
	zl = zerolog.New(zerolog.MultiLevelWriter(zl, fh)).With().Timestamp().Logger()
	logger.Info("Logging to", "file", fh.Name())
	return fh.Close, nil
}

func ensureFilename(fn string, out bool) (string, bool) {
	if !(fn == "" || fn == "-") {
		return fn, false
	}
	fn = filepath.Join(converter.Workdir,
		strconv.Itoa(os.Getpid())+"-"+strconv.Itoa(rand.Int())) //nolint:gas
	fmt.Fprintf(os.Stderr, "fn=%s out? %t\n", fn, out)
	if out {
		return fn, true
	}
	fh, err := os.Create(fn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open temp file %s: %s\n", fn, err)
		os.Exit(4)
	}
	if _, err = io.Copy(fh, os.Stdin); err != nil {
		fmt.Fprintf(os.Stderr, "error writing stdout to %s: %s\n", fn, err)
		os.Exit(5)
	}
	return fn, true
}

func openInOut(fn string, out bool) (*os.File, error) {
	if fn == "" || fn == "-" {
		if out {
			return os.Stdout, nil
		}
		return os.Stdin, nil
	}
	var f *os.File
	var err error
	if out {
		f, err = os.Create(fn)
	} else {
		f, err = os.Open(fn)
	}
	if err != nil {
		return nil, fmt.Errorf("file=%s: %w", fn, err)
	}
	return f, nil
}

func openIn(fn string) (*os.File, error) {
	return openInOut(fn, false)
}

func openOut(fn string) (*os.File, error) {
	return openInOut(fn, true)
}

func isDir(fn string) bool {
	fi, err := os.Stat(fn)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

var _ = tufclient.Destination((*downloadFile)(nil))

type downloadFile struct {
	*os.File
}

func (f *downloadFile) Delete() error {
	if f == nil || f.File == nil {
		return nil
	}
	f.File.Close()
	return os.Remove(f.File.Name())
}
