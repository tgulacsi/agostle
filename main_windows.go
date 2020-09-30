// +build windows

// Copyright 2017 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"gopkg.in/tylerb/graceful.v1"

	"github.com/kardianos/service"
	"github.com/tgulacsi/agostle/converter"
)

//gvi.sh: goversioninfo -product-version=$(git log --format=oneline -n 1 HEAD | cut -d\   -f1)
//go:generate /bin/sh -c ./gvi.sh

const exeName = "agostle.exe"

func init() {
	topCmd = []string{"tasklist", "/v", "/fi", "USERNAME eq " + os.Getenv("USER")}

	serviceCmd := app.Command("service", "manage Windows service")

	addcmd := func(todo string) {
		cmd := serviceCmd.Command(todo, todo+" service")
		args := cmd.Arg("args", "arguments").Strings()
		commands[cmd.FullCommand()] = func(ctx context.Context) error {
			return doServiceWindows(todo, *args)
		}
	}
	for _, todo := range []string{"install", "remove", "run", "start", "stop"} {
		addcmd(todo)
	}
}

var _ = service.Interface((*program)(nil))

type program struct {
	service.Logger
	*graceful.Server
}

func (p *program) Start(S service.Service) error {
	logger.Log("msg", "starting", "service", S)
	if p.Logger != nil {
		_ = p.Logger.Info("Starting service")
	}
	go p.run()
	return nil
}

func (p *program) run() {
	p.Server = newHTTPServer(listenAddr, false)
	logger.Log("msg", "run")
	if err := p.Server.ListenAndServe(); err != nil {
		logger.Log("error", err)
		os.Exit(1)
	}
}

func (p *program) Stop(S service.Service) error {
	logger.Log("msg", "stopping", "service", S)
	if p.Logger != nil {
		_ = p.Logger.Info("Stopping service")
	}
	p.Server.Stop(10 * time.Second)
	return nil
}

func doServiceWindows(todo string, args []string) error {
	if todo == "" {
		todo = "run"
	}
	var short = strings.TrimSuffix(exeName, ".exe")
	if len(args) > 0 && args[0] != "" {
		short = args[0]
	}

	var capShort = strings.ToUpper(short[:1]) + short[1:]
	var name = capShort + " HTTP service"

	p := &program{}
	s, err := service.New(p, &service.Config{
		Name:             short,
		DisplayName:      name,
		Description:      capShort + " converts anything to PDF through HTTP",
		Arguments:        []string{"--config=" + configFile, "service", "run"},
		WorkingDirectory: converter.Workdir,
	})
	if err != nil {
		return fmt.Errorf("start service %s: %w", name, err)
	}
	errs := make(chan error, 5)
	if p.Logger, err = s.Logger(errs); err != nil {
		return fmt.Errorf("get logger: %w", err)
	}
	go func() {
		for err := range errs {
			if err == nil {
				continue
			}
			logger.Log("error", err)
		}
	}()

	switch todo {
	case "install":
		if err = s.Install(); err != nil {
			return fmt.Errorf("install: %w", err)
		}
		logger.Log("msg", "Service "+name+" installed.")
	case "remove":
		if err = s.Uninstall(); err != nil {
			return fmt.Errorf("remove: %w", err)
		}
		logger.Log("msg", "Service "+name+" removed.")
	case "run":
		logger.Log("msg", "running", "service", name)
		if err = s.Run(); err != nil {
			err = fmt.Errorf("run %s: %w", name, err)
			if p.Logger != nil {
				_ = p.Logger.Error(name + " failed: " + err.Error())
			}
			return err
		}
	case "start":
		if err = s.Start(); err != nil {
			return fmt.Errorf("start %s: %w", name, err)
		}
		logger.Log("msg", "Service "+name+" started.")
	case "stop":
		if err = s.Stop(); err != nil {
			return fmt.Errorf("stop %s: %w", name, err)
		}
		logger.Log("msg", "Service "+name+" stopped.")
	default:
		return fmt.Errorf("unknown service %s", todo)
	}
	return nil
}

func getListeners() []net.Listener {
	return nil
}
