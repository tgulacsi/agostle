// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	stdlog "log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"context"

	tufclient "github.com/flynn/go-tuf/client"
	tufdata "github.com/flynn/go-tuf/data"
	"github.com/go-kit/kit/log"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/kardianos/osext"
	"github.com/tgulacsi/agostle/converter"
	"github.com/tgulacsi/go/i18nmail"
	"github.com/tgulacsi/go/loghlp/kitloghlp"

	"github.com/pkg/errors"
)

// go:generate sh -c "overseer-bindiff printkeys --go-out agostle-keyring.gpg >overseer_keyring.go"

const defaultUpdateURL = "https://www.unosoft.hu/tuf"

var (
	swLogger = &log.SwapLogger{}
	logger   = log.Logger(kitloghlp.Stringify{Logger: swLogger})
	ctx      = context.Background()
)

func init() {
	logger = log.With(logger, "t", log.DefaultTimestamp, "caller", log.DefaultCaller())
	swLogger.Swap(log.NewLogfmtLogger(os.Stderr))
	stdlog.SetFlags(0)
	stdlog.SetOutput(log.NewStdlibAdapter(logger))

	converter.Logger = log.With(logger, "lib", "converter")

	sLog := stdlog.New(log.NewStdlibAdapter(log.With(logger, "lib", "i18nmail")), "", 0)
	i18nmail.Debugf, i18nmail.Infof = sLog.Printf, sLog.Printf

}

func main() {
	if err := Main(); err != nil {
		logger.Log("error", err)
		os.Exit(1)
	}
}

var app = kingpin.New("agostle", "agostle is an \"apostle\" which turns everything to PDF")
var (
	configFile, listenAddr string
)

func Main() error {
	updateURL := defaultUpdateURL
	var (
		verbose, leaveTempFiles bool
		concurrency             int
		timeout                 time.Duration
		logFile                 string
	)

	app.Flag("update-url", "URL to download updates from (with GOOS and GOARCH template vars)").Default(updateURL).StringVar(&updateURL)
	app.Flag("leave-tempfiles", "leave tempfiles?").Short('x').BoolVar(&leaveTempFiles)
	app.Flag("verbose", "verbose logging").Short('v').BoolVar(&verbose)
	app.Flag("concurrency", "number of childs start in parallel").Short('C').Default(strconv.Itoa(converter.Concurrency)).IntVar(&concurrency)
	app.Flag("timeout", "timeout for external programs").Default("10m").DurationVar(&timeout)
	app.Flag("config", "config file (TOML)").Short('c').ExistingFileVar(&configFile)
	app.Flag("logfile", "logfile").StringVar(&logFile)
	app.HelpFlag.Short('h')

	var updateRootJSON, updateRootKeys string
	updateCmd := app.Command("update", "update binary to the latest release")
	updateCmd.Flag("root-keys-string", "CONTENTS of root.json for TUF update").Default(defaultRootKeys).StringVar(&updateRootKeys)
	updateCmd.Flag("root-keys-file", "PATH of root.json for TUF update").Default(updateRootJSON).StringVar(&updateRootJSON)

	var savereq bool
	var regularUpdates time.Duration
	serveCmd := app.Command("server", "serve HTTP").Alias("serve")
	serveCmd.Flag("regular-updates", "do regular updates at this interval").DurationVar(&regularUpdates)
	serveCmd.Flag("savereq", "save requests").BoolVar(&savereq)
	serveCmd.Arg("addr", "addr.to.listen.on:port").Default("").StringVar(&listenAddr)

	todo, err := app.Parse(os.Args[1:])
	if err != nil {
		return err
	}

	Log := logger.Log
	var closeLogfile func() error

	if closeLogfile, err = logToFile(logFile); err != nil {
		return err
	}
	if !verbose {
		i18nmail.Debugf = nil
	}
	Log("leave_tempfiles?", leaveTempFiles)
	converter.LeaveTempFiles = leaveTempFiles
	converter.Concurrency = concurrency
	if configFile == "" {
		if self, execErr := osext.Executable(); execErr != nil {
			Log("msg", "Cannot determine executable file name", "error", execErr)
		} else {
			ini := filepath.Join(filepath.Dir(self), "agostle.ini")
			f, iniErr := os.Open(ini)
			if iniErr != nil {
				Log("msg", "Cannot open config", "file", ini, "error", iniErr)
			} else {
				_ = f.Close()
				configFile = ini
			}
		}
	}
	Log("msg", "Loading config", "file", configFile)
	if err = converter.LoadConfig(ctx, configFile); err != nil {
		Log("msg", "Parsing config", "file", configFile, "error", err)
		return err
	}
	if timeout > 0 && timeout != *converter.ConfChildTimeout {
		Log("msg", "Setting timeout", "from", *converter.ConfChildTimeout, "to", timeout)
		*converter.ConfChildTimeout = timeout
	}
	if closeLogfile == nil {
		if closeLogfile, err = logToFile(*converter.ConfLogFile); err != nil {
			Log("error", err)
		}
	}

	sortBeforeMerge = *converter.ConfSortBeforeMerge
	Log("msg", "commands",
		"pdftk", *converter.ConfPdftk,
		"loffice", *converter.ConfLoffice,
		"gm", *converter.ConfGm,
		"gs", *converter.ConfGs,
		"pdfclean", *converter.ConfPdfClean,
		"wkhtmltopdf", *converter.ConfWkhtmltopdf,
	)
	Log("msg", "parameters",
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
			Log("msg", "close log file", "error", closeLogfile())
		}()
	}

	switch todo {
	case updateCmd.FullCommand():
		self, err := os.Executable()
		if err != nil {
			return err
		}
		logger.Log("msg", "update", "from", updateURL)
		remote, err := tufclient.HTTPRemoteStore(updateURL, nil)
		if err != nil {
			return err
		}
		var rootKeysSrc io.Reader = strings.NewReader(updateRootKeys)
		if updateRootJSON != "" {
			logger.Log("msg", "using root keys", "from", updateRootJSON)
			b, readErr := ioutil.ReadFile(updateRootJSON)
			if readErr != nil {
				return readErr
			}
			rootKeysSrc = bytes.NewReader(b)
		}
		var rootKeys []*tufdata.Key
		if json.NewDecoder(rootKeysSrc).Decode(&rootKeys); err != nil {
			return err
		}
		tc := tufclient.NewClient(tufclient.MemoryLocalStore(), remote)
		if err := tc.Init(rootKeys, len(rootKeys)); err != nil {
			return errors.Wrap(err, "Init")
		}
		targets, err := tc.Update()
		if err != nil {
			return errors.Wrap(err, "Update")
		}
		for f := range targets {
			logger.Log("target", f)
		}

		destFh, err := os.Create(
			filepath.Join(filepath.Dir(self), "."+filepath.Base(self)+".new"),
		)
		if err != nil {
			return err
		}
		defer destFh.Close()
		logger.Log("msg", "download", "to", destFh.Name())
		dest := &downloadFile{File: destFh}
		if err := tc.Download(
			strings.Replace(strings.Replace(
				"/agostle/{{GOOS}}_{{GOARCH}}",
				"{{GOOS}}", runtime.GOOS, -1),
				"{{GOARCH}}", runtime.GOARCH, -1),
			dest,
		); err != nil {
			return errors.Wrap(err, "Download")
		}
		_ = os.Chmod(destFh.Name(), 0775)

		old := filepath.Join(filepath.Dir(self), "."+filepath.Base(self)+".old")
		logger.Log("msg", "rename", "from", self, "to", old)
		if err := os.Rename(self, old); err != nil {
			return err
		}
		logger.Log("msg", "rename", "from", destFh.Name(), "to", self)
		if err := os.Rename(destFh.Name(), self); err != nil {
			logger.Log("error", err)
		} else {
			os.Remove(old)
		}

		return nil

	case serveCmd.FullCommand():
		if listenAddr == "" {
			listenAddr = *converter.ConfListenAddr
		}

		return errors.Wrap(
			newHTTPServer(listenAddr, savereq).ListenAndServe(),
			listenAddr)

	}
	f, ok := commands[todo]
	if !ok {
		return errors.New("unknown command " + todo)
	}
	return f(ctx)
}

var commands = make(map[string]func(context.Context) error)

func logToFile(fn string) (func() error, error) {
	if fn == "" {
		return nil, nil
	}
	fh, err := os.OpenFile(fn, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		logger.Log("error", err)
		return nil, errors.Wrap(err, fn)
	}
	logger.Log("msg", "Will log to", "file", fh.Name())
	swLogger.Swap(log.NewLogfmtLogger(io.MultiWriter(os.Stderr, fh)))
	logger.Log("msg", "Logging to", "file", fh.Name())
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
		return nil, errors.Wrapf(err, "file="+fn)
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
