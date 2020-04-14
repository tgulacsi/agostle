// +build !windows

// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"net"

	"github.com/coreos/go-systemd/v22/activation"
)

func getListeners() []net.Listener {
	listeners, _ := activation.Listeners()
	for i := range listeners {
		if listeners[i] == nil {
			listeners[i] = listeners[0]
			listeners = listeners[1:]
			i--
		}
	}
	return listeners
}
