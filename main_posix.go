//go:build !windows
// +build !windows

// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"net"
	"time"

	"github.com/coreos/go-systemd/v22/activation"
	"github.com/coreos/go-systemd/v22/daemon"
)

func getListeners() []net.Listener {
	listeners, _ := activation.Listeners()
	for i := 0; i < len(listeners); i++ {
		if listeners[i] == nil {
			listeners[i] = listeners[0]
			listeners = listeners[1:]
			i--
		}
	}
	return listeners
}

func sdNotify(done <-chan struct{}) error {
	notify := func(message string) {
		logger.Info("sdNotify", "message", message)
		if _, err := daemon.SdNotify(false, message); err != nil {
			logger.Error(message, "error", err)
		}
	}
	logger.Info("start stNotify")
	defer logger.Warn("finish sdNotify")
	notify(daemon.SdNotifyReady)
	dur, err := daemon.SdWatchdogEnabled(true)
	if err != nil || dur == 0 {
		logger.Warn("watchdog", "dur", dur, "error", err)
		<-done
	} else {
		logger.Info("watchdog", "dur", dur.String())
		ticker := time.NewTicker(dur / 2)
	Loop:
		for {
			select {
			case <-ticker.C:
				notify(daemon.SdNotifyWatchdog)
			case <-done:
				ticker.Stop()
				break Loop
			}
		}
	}
	notify(daemon.SdNotifyStopping)
	return nil
}
