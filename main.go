// Copyright 2013 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	stdlog "log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/go-kit/kit/log"
	"github.com/jpillora/overseer"
	"github.com/tgulacsi/overseer-bindiff/fetcher"

	"github.com/kardianos/osext"
	"github.com/spf13/cobra"
	"github.com/tgulacsi/agostle/converter"
	"github.com/tgulacsi/go/i18nmail"
	"github.com/tgulacsi/go/loghlp/kitloghlp"

	"github.com/pkg/errors"
)

//go:generate sh -c "overseer-bindiff printkeys --go-out agostle-keyring.gpg >overseer_keyring.go"

const defaultUpdateURL = "https://www.unosoft.hu/agostle"

var (
	swLogger = &log.SwapLogger{}
	logger   = log.NewContext(kitloghlp.Stringify{swLogger})
	ctx      = context.Background()
)

func init() {
	logger = logger.With("t", log.DefaultTimestamp, "caller", log.Caller(4))
	swLogger.Swap(log.NewLogfmtLogger(os.Stderr))
	stdlog.SetFlags(0)
	stdlog.SetOutput(log.NewStdlibAdapter(logger))

	converter.Logger = logger.With("lib", "converter")

	sLog := stdlog.New(log.NewStdlibAdapter(logger.With("lib", "i18nmail")), "", 0)
	i18nmail.Debugf, i18nmail.Infof = sLog.Printf, sLog.Printf

	fetcher.Logf = stdlog.New(log.NewStdlibAdapter(logger.With("lib", "fetcher")), "", 0).Printf
}

func getListenAddr(args []string) string {
	for _, x := range args {
		if x == "" {
			break
		}
		return x
	}
	return *converter.ConfListenAddr
}

var agostleCmd = &cobra.Command{
	Use:   "agostle",
	Short: "agostle is an \"apostle\" which turns everything to PDF",
}

func main() {
	updateURL := defaultUpdateURL
	var (
		verbose, leaveTempFiles bool
		concurrency             int
		timeout                 time.Duration
		configFile, logFile     string
	)
	p := agostleCmd.PersistentFlags()
	p.StringVarP(&updateURL, "update-url", "", updateURL, "URL to download updates from (with GOOS and GOARCH template vars)")
	p.BoolVarP(&leaveTempFiles, "leave-tempfiles", "x", false, "leave tempfiles?")
	p.BoolVarP(&verbose, "verbose", "v", false, "verbose logging")
	p.IntVarP(&concurrency, "concurrency", "C", converter.Concurrency, "number of childs start in parallel")
	p.DurationVar(&timeout, "timeout", 10*time.Minute, "timeout for external programs")
	p.StringVarP(&configFile, "config", "c", "", "config file (TOML)")
	p.StringVarP(&logFile, "logfile", "", "", "logfile")

	Log := logger.Log
	var closeLogfile func() error
	cobra.OnInitialize(func() {
		var err error
		if closeLogfile, err = logToFile(logFile); err != nil {
			Log("error", err)
			os.Exit(1)
		}
		if !verbose {
			i18nmail.Debugf = nil
		}
		Log("leave_tempfiles?", leaveTempFiles)
		converter.LeaveTempFiles = leaveTempFiles
		converter.Concurrency = concurrency
		if configFile == "" {
			if self, err := osext.Executable(); err != nil {
				Log("msg", "Cannot determine executable file name", "error", err)
			} else {
				ini := filepath.Join(filepath.Dir(self), "agostle.ini")
				f, err := os.Open(ini)
				if err != nil {
					Log("msg", "Cannot open config", "file", ini, "error", err)
				} else {
					_ = f.Close()
					configFile = ini
				}
			}
		}
		Log("msg", "Loading config", "file", configFile)
		if err := converter.LoadConfig(configFile); err != nil {
			Log("msg", "Parsing config", "file", configFile, "error", err)
			os.Exit(1)
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
	})
	if closeLogfile != nil {
		defer func() {
			Log("msg", "close log file", "error", closeLogfile())
		}()
	}

	var keyRing string
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "update binary to the latest release",
		Run: func(cmd *cobra.Command, args []string) {
			if keyRing != "" {
				fh, err := os.Open(keyRing)
				if err != nil {
					Log("msg", "open "+keyRing, "error", err)
					os.Exit(3)
				}
				keyring = readKeyring(fh)
				fh.Close()
			}
			overseer.Run(overseer.Config{
				Debug:   true,
				Program: func(state overseer.State) {},
				Fetcher: &fetcher.HTTPSelfUpdate{
					URL:      updateURL,
					Interval: 1 * time.Second,
					Keyring:  keyring,
				},
				NoRestart: true,
			})
		},
	}
	updateCmd.Flags().StringVar(&keyRing, "keyring", "", "keyring for decrypting the updates")
	agostleCmd.AddCommand(updateCmd)

	{
		var savereq bool
		var regularUpdates time.Duration

		serveCmd := &cobra.Command{
			Use:   "serve",
			Short: "serve HTTP",
			Long:  "serve [-savereq] addr.to.listen.on:port",
			Run: func(cmd *cobra.Command, args []string) {
				addr := getListenAddr(args)
				if updateURL == "" || regularUpdates == 0 {
					Log("msg", newHTTPServer(addr, savereq).ListenAndServe())
					os.Exit(1)
				}
				overseer.Run(overseer.Config{
					Debug: true,
					Program: func(state overseer.State) {
						if state.Listener == nil {
							Log("msg", "overseer gave nil listener! Will try "+addr)
							Log("msg", newHTTPServer(addr, savereq).ListenAndServe())
							os.Exit(1)
						}
						startHTTPServerListener(state.Listener, savereq)
					},
					Address: addr,
					Fetcher: &fetcher.HTTPSelfUpdate{
						URL:      updateURL,
						Interval: regularUpdates,
					},
				})
			},
		}

		serveCmd.Flags().DurationVar(&regularUpdates, "regular-updates", 0, "do regular updates at this interval")
		serveCmd.Flags().BoolVar(&savereq, "savereq", false, "save requests")
		agostleCmd.AddCommand(serveCmd)
	}

	if len(os.Args) == 1 {
		overseer.SanityCheck()
	}
	if err := agostleCmd.Execute(); err != nil {
		Log("error", err)
		os.Exit(1)
	}
}

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
		strconv.Itoa(os.Getpid())+"-"+strconv.Itoa(rand.Int()))
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
