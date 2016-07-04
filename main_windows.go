// +build windows

// Copyright 2013 The Agostle Authors. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"strings"
	"time"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	"github.com/tgulacsi/agostle/converter"
)

//gvi.sh: goversioninfo -product-version=$(git log --format=oneline -n 1 HEAD | cut -d\   -f1)
//go:generate /bin/sh -c ./gvi.sh

const exeName = "agostle.exe"

func init() {
	topCmd = []string{"tasklist", "/v", "/fi", "USERNAME eq " + os.Getenv("USER")}

	serviceCmd := &cobra.Command{
		Use: "service",
	}
	agostleCmd.AddCommand(serviceCmd)

	addcmd := func(todo string) {
		serviceCmd.AddCommand(&cobra.Command{
			Use: todo,
			Run: func(cmd *cobra.Command, args []string) {
				doServiceWindows(todo, args)
			},
		})
	}
	for _, todo := range []string{"install", "remove", "run", "start", "stop"} {
		addcmd(todo)
	}
}

var _ = service.Interface((*program)(nil))

type program struct {
	service.Logger
	Server
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
	p.Server = newHTTPServer(getListenAddr(nil), false)
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

func doServiceWindows(todo string, args []string) {
	if todo == "" {
		todo = "run"
	}
	var short = strings.TrimSuffix(exeName, ".exe")
	if len(args) > 0 && args[0] != "" {
		short = args[0]
	}

	var capShort = strings.ToUpper(short[:1]) + short[1:]
	var name = capShort + " HTTP service"

	configFile, err := agostleCmd.PersistentFlags().GetString("config")
	if err != nil {
		logger.Log("msg", "get config file", "error", err)
		os.Exit(1)
	}
	p := &program{}
	s, err := service.New(p, &service.Config{
		Name:             short,
		DisplayName:      name,
		Description:      capShort + " converts anything to PDF through HTTP",
		Arguments:        []string{"--config=" + configFile, "service", "run"},
		WorkingDirectory: converter.Workdir,
	})
	if err != nil {
		logger.Log("msg", "unable to start", "service", name, "error", err)
		os.Exit(1)
	}
	errs := make(chan error, 5)
	if p.Logger, err = s.Logger(errs); err != nil {
		logger.Log("msg", "get logger", "error", err)
		os.Exit(1)
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
			logger.Log("msg", "Failed to install", "error", err)
			os.Exit(1)
		}
		logger.Log("msg", "Service "+name+" installed.")
	case "remove":
		if err = s.Uninstall(); err != nil {
			logger.Log("msg", "Failed to remove", "error", err)
			os.Exit(1)
		}
		logger.Log("msg", "Service "+name+" removed.")
	case "run":
		logger.Log("msg", "running", "service", name)
		if err = s.Run(); err != nil {
			logger.Log("msg", "Running service "+name+" failed.", "error", err)
			if p.Logger != nil {
				_ = p.Logger.Error(name + " failed: " + err.Error())
			}
			os.Exit(1)
		}
	case "start":
		if err = s.Start(); err != nil {
			logger.Log("msg", "Failed to start", "error", err)
			os.Exit(1)
		}
		logger.Log("msg", "Service "+name+" started.")
	case "stop":
		if err = s.Stop(); err != nil {
			logger.Log("msg", "Failed to stop", "error", err)
			os.Exit(1)
		}
		logger.Log("msg", "Service "+name+" stopped.")
	default:
		logger.Log("msg", "unknown service "+todo)
		os.Exit(1)
	}
}
