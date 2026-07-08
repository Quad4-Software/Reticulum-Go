// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !windows && !tinygo

package main

import (
	"os"
	"os/signal"
	"syscall"

	"quad4/reticulum-go/internal/config"
	"quad4/reticulum-go/pkg/debug"
)

func startSIGHUPReload(r *Reticulum) {
	go func() {
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		for range hup {
			path := r.config.ConfigPath
			if path == "" {
				p, err := config.GetConfigPath()
				if err != nil {
					debug.Log(debug.DebugCritical, "SIGHUP reload: config path", "error", err)
					continue
				}
				path = p
			}
			newCfg, err := config.LoadConfig(path)
			if err != nil {
				debug.Log(debug.DebugCritical, "SIGHUP reload: load config", "error", err)
				continue
			}
			if err := r.ReloadInterfaces(newCfg); err != nil {
				debug.Log(debug.DebugCritical, "ReloadInterfaces", "error", err)
			} else {
				r.config = newCfg
				debug.Log(debug.DebugInfo, "Reloaded interfaces from config", "path", path)
			}
		}
	}()
}

func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
