// Copyright 2013 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package converter

import (
	"net"
	"strconv"
	"time"
)

// PortLock is a locker which locks by binding to a port on the loopback IPv4 interface
type PortLock struct {
	hostport string
	ln       net.Listener
}

// NewPortLock returns a lock for port
func NewPortLock(port int) *PortLock {
	return &PortLock{hostport: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}
}

// Lock locks on port
func (p *PortLock) Lock() {
	var err error
	t := 1 * time.Second
	for {
		if p.ln, err = net.Listen("tcp", p.hostport); err == nil {
			return
		}
		Log("msg", "spinning lock hostport", "hostport", p.hostport, "error", err)
		time.Sleep(t)
		t = time.Duration(float32(t) * 1.2)
	}
}

// Unlock unlocks the port lock
func (p *PortLock) Unlock() {
	if p.ln != nil {
		_ = p.ln.Close()
	}
}
