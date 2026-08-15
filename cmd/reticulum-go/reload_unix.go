// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !windows && !tinygo

package main

import (
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"quad4/reticulum-go/internal/config"
	"quad4/reticulum-go/pkg/debug"
)

func startSIGHUPReload(r *Reticulum, opts daemonOptions) {
	go func() {
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		for range hup {
			if runtime.GOOS == "freebsd" && r.config != nil && r.config.EnableSandbox {
				debug.Log(debug.DebugInfo, "SIGHUP reload: re-exec under CapEnter sandbox")
				if err := r.StopDaemon(); err != nil {
					debug.Log(debug.DebugCritical, "SIGHUP re-exec: stop", "error", err)
					continue
				}
				if err := reexecDaemon(); err != nil {
					debug.Log(debug.DebugCritical, "SIGHUP re-exec failed", "error", err)
				}
				continue
			}
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
				applyDaemonLogging(newCfg, daemonOptions{DebugLevel: -1, JSONLogs: opts.JSONLogs})
				debug.Log(debug.DebugInfo, "Reloaded interfaces from config", "path", path)
			}
		}
	}()
}

func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func reexecDaemon() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	argv := make([]string, len(os.Args))
	copy(argv, os.Args)
	argv[0] = exe
	return syscall.Exec(exe, argv, os.Environ())
}
