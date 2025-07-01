// Copyright 2017, 2022 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

// Needed: /email/convert?splitted=1&errors=1&id=xxx Accept: images/gif
//  /pdf/merge Accept: application/zip

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/kardianos/osext"
	"github.com/tgulacsi/go/version"
)

type statInfo struct {
	last               time.Time
	mem                *runtime.MemStats
	startedAt, version string
	top                []byte
	mtx                sync.Mutex
}

var stats = new(statInfo)
var (
	topOut      = bytes.NewBuffer(make([]byte, 0, 4096))
	topCmd      = []string{"top", "-b", "-n1", "-s", "-S", "-u", ""} // will be different on windows - see main_windows.go
	onceOnStart = new(sync.Once)
)

func onStart() {
	var err error
	if self, err = osext.Executable(); err != nil {
		logger.Error("error getting the path for self", "error", err)
	} else {
		var self2 string
		if self2, err = filepath.Abs(self); err != nil {
			logger.Error("error getting the absolute path", "for", self, "error", err)
		} else {
			self = self2
		}
	}

	var uname string
	if u, e := user.Current(); e != nil {
		logger.Error("cannot get current user", "error", e)
		uname = os.Getenv("USER")
	} else {
		uname = u.Username
	}
	i := len(topCmd) - 1
	topCmd[i] = topCmd[i] + uname

	stats.startedAt = time.Now().Format(time.RFC3339)

	http.DefaultServeMux.Handle("/", http.HandlerFunc(statusPage))
}

// getTopOut returns the output of the topCmd - shall be protected with a mutex
func getTopOutput() ([]byte, error) {
	topOut.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := exec.CommandContext(ctx, topCmd[0], topCmd[1:]...)
	cmd.Stdout = topOut
	cmd.Stderr = os.Stderr
	e := cmd.Run()
	if e != nil {
		logger.Error("error calling", "cmd", topCmd, "error", e)
		fmt.Fprintf(topOut, "\n\nerror calling %s: %s\n", topCmd, e)
	}
	return topOut.Bytes(), e
}

// fill fills the stat iff the current one is stale
func (st *statInfo) fill() {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	now := time.Now()
	if st.mem == nil {
		st.mem = new(runtime.MemStats)
		st.version = runtime.Version()
	} else if now.Sub(st.last) <= 5*time.Second {
		return
	}
	st.last = now
	runtime.ReadMemStats(st.mem)
	top, err := getTopOutput()
	if err != nil {
		logger.Error("error calling top", "error", err)
	} else {
		st.top = bytes.Replace(top, []byte("\n"), []byte("\n    "), -1)
	}
}

func statusPage(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "/favicon.ico" {
		http.Error(w, "", 404)
		return
	}
	stats.fill()
	w.Header().Add("Content-Type", "text/html")
	w.WriteHeader(200)
	// nosemgrep: go.lang.security.audit.xss.no-fprintf-to-responsewriter.no-fprintf-to-responsewriter
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
  <head><title>Agostle</title></head>
  <body>
    <h1>Agostle</h1>
    <p>%s</p>
    <p>%s compiled with Go version %s</p>
    <p>%d started at %s<br/>
    Allocated: %.03fMb (Sys: %.03fMb)</p>

    <p><a href="/_admin/stop">Stop</a> (hopefully supervisor runit will restart).</p>

    <h2>Top</h2>
    <pre>    `,
		version.Main(),
		self, stats.version,
		os.Getpid(), stats.startedAt,
		float64(stats.mem.Alloc)/1024/1024, float64(stats.mem.Sys)/1024/1024)
	//io.WriteString(w, stats.top)
	// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
	_, _ = w.Write(stats.top)
	_, _ = io.WriteString(w, `</pre></body></html>`)
}
