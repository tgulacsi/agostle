// Copyright 2017, 2023 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/UNO-SOFT/zlog/v2"
	"github.com/peterbourgon/ff/v3/ffcli"
	tufclient "github.com/theupdateframework/go-tuf/client"

	"github.com/kardianos/osext"
	"github.com/rogpeppe/retry"
	"github.com/tgulacsi/agostle/converter"
	"github.com/tgulacsi/go/i18nmail"
	"github.com/tgulacsi/go/version"
	"golang.org/x/sync/errgroup"
)

// go : generate sh -c "overseer-bindiff printkeys --go-out agostle-keyring.gpg >overseer_keyring.go"

const defaultUpdateURL = "https://www.unosoft.hu/tuf"

var (
	verbose = zlog.VerboseVar(1)
	zl      = zlog.NewMultiHandler(zlog.MaybeConsoleHandler(&verbose, zlog.NewSyncWriter(os.Stderr)))
	logger  = zlog.NewLogger(zl).SLog()
)

func main() {
	if err := Main(); err != nil {
		logger.Error("Main", "error", err)
		os.Exit(1)
	}
}

var (
	configFile, listenAddr string

	subcommands []*ffcli.Command
)

func newFlagSet(name string) *flag.FlagSet { return flag.NewFlagSet(name, flag.ContinueOnError) }

func Main() error {
	slog.SetDefault(logger)

	// Set limit to 4GiB
	pid := os.Getpid()
	for _, l := range []struct {
		Resource int
		Limit    uint64
	}{
		{Resource: unix.RLIMIT_AS, Limit: 4 << 30},
		{Resource: unix.RLIMIT_DATA, Limit: 4 << 30},
	} {
		var old unix.Rlimit
		if err := unix.Prlimit(
			pid, l.Resource,
			&unix.Rlimit{Cur: l.Limit, Max: l.Limit}, &old,
		); err != nil {
			return err
		}
	}

	updateURL := defaultUpdateURL
	var (
		leaveTempFiles        bool
		concurrency           int
		timeout               time.Duration
		logFile, gotenbergURL string
	)

	fs := newFlagSet("agostle")
	fs.StringVar(&updateURL, "update-url", updateURL, "URL to download updates from (with GOOS and GOARCH template vars)")
	fs.BoolVar(&leaveTempFiles, "x", false, "leave tempfiles?")
	fs.Var(&verbose, "v", "verbose logging")
	fs.IntVar(&concurrency, "concurrency", converter.Concurrency, "number of childs start in parallel")
	fs.DurationVar(&timeout, "timeout", 10*time.Minute, "timeout for external programs")
	fs.StringVar(&configFile, "config", "", "config file (TOML)")
	fs.StringVar(&logFile, "logfile", "", "logfile")
	fs.StringVar(converter.ConfGotenbergURL, "gotenberg", "", "gotenberg service URL")
	fs.Uint64Var(converter.ConfMaxSubprocMemoryBytes, "max-subproc-mem-bytes", converter.DefaultMaxSubprocMemoryBytes, "maximum subprocess memory limit")
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
				if rootKeySrc, readErr = os.ReadFile(updateRootJSON); readErr != nil {
					return readErr
				}
			}
			tc := tufclient.NewClient(tufclient.MemoryLocalStore(), remote)
			if err := tc.Init(rootKeySrc); err != nil {
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
			// nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission
			_ = os.Chmod(destFh.Name(), 0755)

			old := filepath.Join(filepath.Dir(self), "."+filepath.Base(self)+".old")
			logger.Info("rename", "from", self, "to", old)
			if err := os.Rename(self, old); err != nil {
				return err
			}
			logger.Info("rename", "from", destFh.Name(), "to", self)
			if err := os.Rename(destFh.Name(), self); err != nil {
				logger.Error("rename", "from", destFh.Name(), "to", self, "error", err)
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

			go func() {
				strategy := retry.Strategy{Delay: 5 * time.Minute}
				var iter *retry.Iter
				for {
					threshold := time.Now().Add(-time.Hour)
					wd := converter.Workdir
					logger.Info("clear result-*.zip files", "dir", wd, "threshold", threshold)
					dis, _ := os.ReadDir(wd)
					for _, di := range dis {
						bn := di.Name()
						if !(di.Type().IsRegular() &&
							strings.HasPrefix(bn, "result-") &&
							strings.HasSuffix(bn, ".zip")) {
							continue
						}
						if fi, err := di.Info(); err == nil && fi.ModTime().Before(threshold) {
							fn := filepath.Join(wd, bn)
							logger.Info("Remove", "file", fn)
							os.Remove(fn)
						}
					}
					if iter == nil {
						iter = strategy.Start()
					}
					if !iter.Next(ctx.Done()) {
						break
					}
				}
			}()

			grp, grpCtx := errgroup.WithContext(ctx)
			grp.SetLimit(converter.Concurrency)
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
	logger = zlog.NewLogger(zl).SLog()
	converter.SetLogger(logger)
	ctx, cancel := signal.NotifyContext(
		zlog.NewSContext(context.Background(), logger),
		os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if *converter.ConfMutool == "" {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		env := append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		for _, todo := range [][]string{
			{"apt-get", "-y", "update"},
			{"apt-get", "-y", "install", "mupdf-tools"},
		} {
			cmd := exec.CommandContext(ctx, todo[0], todo[1:]...)
			cmd.Env = env
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			logger.Info("try", "command", cmd.Args)
			if err := cmd.Run(); err != nil {
				logger.Warn("try", "command", cmd.Args, "error", err)
			}
		}
		cancel()
		if s, _ := exec.LookPath("mutool"); s != "" {
			*converter.ConfMutool = s
		}
	}

	var closeLogfile func() error

	var err error
	if closeLogfile, err = logToFile(logFile); err != nil {
		return err
	}
	i18nmail.SetLogger(logger.WithGroup("i18nmail"))
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
	go func() { <-ctx.Done(); logger.Info("DONE"); time.Sleep(time.Second); logger.Info("EXIT"); os.Exit(3) }()
	logger.Info("Loading config", "file", configFile)
	if err = converter.LoadConfig(ctx, configFile); err != nil {
		logger.Info("Parsing config", "file", configFile, "error", err)
		return err
	}
	if gotenbergURL != "" {
		*converter.ConfGotenbergURL = gotenbergURL
	}
	if timeout > 0 && timeout != *converter.ConfChildTimeout {
		logger.Info("Setting timeout", "from", *converter.ConfChildTimeout, "to", timeout)
		*converter.ConfChildTimeout = timeout
	}
	if closeLogfile == nil {
		if closeLogfile, err = logToFile(*converter.ConfLogFile); err != nil {
			logger.Error("logToFile", "error", err)
		}
	}

	sortBeforeMerge = *converter.ConfSortBeforeMerge
	logger.Info("commands",
		"gm", *converter.ConfGm,
		"gs", *converter.ConfGs,
		"loffice", *converter.ConfLoffice,
		"mutool", *converter.ConfMutool,
		"pdfclean", *converter.ConfPdfClean,
		"pdftk", *converter.ConfPdftk,
		"wkhtmltopdf", *converter.ConfWkhtmltopdf,
	)
	logger.Info("parameters",
		"sortBeforeMerge", sortBeforeMerge,
		"workdir", converter.Workdir,
		"listen", *converter.ConfListenAddr,
		"childTimeout", *converter.ConfChildTimeout,
		"defaultIsService", *converter.ConfDefaultIsService,
		"logfile", *converter.ConfLogFile,
		"gotenbergURL", *converter.ConfGotenbergURL,
		"version", version.Main(),
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
	// nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission
	fh, err := os.OpenFile(fn, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		logger.Error("open log file", "file", fn, "error", err)
		return nil, fmt.Errorf("%s: %w", fn, err)
	}
	logger.Info("Will log to", "file", fh.Name())
	zl.Add(slog.NewJSONHandler(fh, &zlog.DefaultHandlerOptions.HandlerOptions))
	logger.Info("Logging to", "file", fh.Name())
	return fh.Close, nil
}

func ensureFilename(fn string, out bool) (string, bool) {
	if !(fn == "" || fn == "-") {
		return fn, false
	}
	// nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used
	fn = filepath.Join(converter.Workdir,
		// nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used
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
