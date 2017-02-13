// Copyright 2013 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"os/exec"
	"time"

	"context"

	"github.com/tgulacsi/go/proc"
)

func runWithTimeout(cmd *exec.Cmd) error {
	err := proc.RunWithTimeout(int(*ConfChildTimeout/time.Second), cmd)
	if err != nil {
		Log("msg", "ERROR runWithTimeout", "args", cmd.Args, "error", err)
	}
	return err
}

func runWithContext(ctx context.Context, cmd *exec.Cmd) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	timeout := *ConfChildTimeout
	deadline, ok := ctx.Deadline()
	if ok {
		timeout = deadline.Sub(time.Now())
	}
	seconds := int(timeout / time.Second)
	if seconds <= 0 {
		return cmd.Run()
	}
	return proc.RunWithTimeout(int(timeout/time.Second), cmd)
}
