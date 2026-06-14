// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/reticulumconfig"

	"quad4/reticulum-go/examples/pageserver/internal/server"
)

var (
	configPath          string
	pagesDirFlag        string
	filesDirFlag        string
	nodeNameFlag        string
	announceIntervalMin int
	pageRefreshSec      int
	fileRefreshSec      int
	pageserverLogLevel  int
	identityOverride    string
	disablePageStats    bool
)

func init() {
	flag.StringVar(&configPath, "config", "", "Reticulum config file (default ~/.reticulum-go/config).")
	flag.StringVar(&configPath, "c", "", "Same as -config.")
	flag.StringVar(&pagesDirFlag, "pages-dir", "pages", "Pages directory for /page/.")
	flag.StringVar(&pagesDirFlag, "p", "pages", "Same as -pages-dir.")
	flag.StringVar(&filesDirFlag, "files-dir", "files", "Files directory for /file/.")
	flag.StringVar(&filesDirFlag, "f", "files", "Same as -files-dir.")
	flag.StringVar(&nodeNameFlag, "node-name", server.AppName, "Node display name in announces (max 255 bytes).")
	flag.StringVar(&nodeNameFlag, "n", server.AppName, "Same as -node-name.")
	flag.IntVar(&announceIntervalMin, "announce-interval", 360, "Periodic announces every N whole minutes after boot. Use 0 to disable repeats after the first announce. Default 360 (6 hours).")
	flag.IntVar(&announceIntervalMin, "a", 360, "Same as -announce-interval (minutes).")
	flag.IntVar(&pageRefreshSec, "page-refresh", 0, "Rescan pages directory every N seconds. Use 0 to scan only at startup.")
	flag.IntVar(&pageRefreshSec, "pages-refresh-interval", 0, "Same as -page-refresh (seconds).")
	flag.IntVar(&fileRefreshSec, "file-refresh", 0, "Rescan files directory every N seconds. Use 0 to scan only at startup.")
	flag.IntVar(&fileRefreshSec, "files-refresh-interval", 0, "Same as -file-refresh (seconds).")
	flag.IntVar(&pageserverLogLevel, "log-level", -1, "Log verbosity 1-7. Use -1 to take the level from the config file.")
	flag.StringVar(&identityOverride, "identity", "", "Identity file path (default ~/.reticulum-go/storage/identity).")
	flag.StringVar(&identityOverride, "identity-path", "", "Same as -identity.")
	flag.BoolVar(&disablePageStats, "no-page-stats", false, "Disable built-in page view stats and stop recording views. A static pages/__pageviews.mu can still be served.")
}

func main() {
	flag.Parse()
	cfg, err := loadOrInitConfig(configPath)
	if err != nil {
		applyPageserverLogLevel(nil)
		debug.Init()
		debug.GetLogger().Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}
	applyPageserverLogLevel(cfg)
	debug.Init()
	if debug.GetDebugLevel() > debug.DebugCritical {
		debug.Log(debug.DebugCritical, "Initializing Reticulum", "debug_level", debug.GetDebugLevel())
	}
	if debug.GetDebugLevel() >= debug.DebugError {
		debug.Log(debug.DebugError, "Configuration loaded", "path", cfg.ConfigPath)
	}

	opts := server.Options{
		PagesDir:                pagesDirFlag,
		FilesDir:                filesDirFlag,
		PageRefreshInterval:     time.Duration(pageRefreshSec) * time.Second,
		FileRefreshInterval:     time.Duration(fileRefreshSec) * time.Second,
		AnnounceIntervalMinutes: announceIntervalMin,
		IdentityFileOverride:    identityOverride,
		NodeDisplayName:         server.ClampDisplayName(nodeNameFlag, server.AppName),
		DisablePageStats:        disablePageStats,
	}

	r, err := server.NewReticulum(cfg, opts)
	if err != nil {
		debug.GetLogger().Error("Failed to create Reticulum instance", "error", err)
		os.Exit(1)
	}

	go r.MonitorInterfaces()

	handler := server.NewAnnounceHandler(r, []string{"*"})
	r.Transport().RegisterAnnounceHandler(handler)

	if err := r.Start(); err != nil {
		debug.GetLogger().Error("Failed to start Reticulum", "error", err)
		os.Exit(1)
	}

	server.PrintStartupSummary(r)

	sigChan := make(chan os.Signal, 1)
	sigs := []os.Signal{os.Interrupt}
	if runtime.GOOS != "windows" {
		sigs = append(sigs, syscall.SIGTERM)
	}
	signal.Notify(sigChan, sigs...)
	<-sigChan

	debug.Log(debug.DebugCritical, "Shutting down...")
	if err := r.Stop(); err != nil {
		debug.Log(debug.DebugCritical, "Error during shutdown", "error", err)
	}
	debug.Log(debug.DebugCritical, "Goodbye!")
}

func flagWasSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func loadOrInitConfig(override string) (*common.ReticulumConfig, error) {
	path := override
	if path == "" {
		p, err := reticulumconfig.GetConfigPath()
		if err != nil {
			return nil, fmt.Errorf("resolve default config path: %w", err)
		}
		path = p
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := reticulumconfig.CreateDefaultConfig(path); err != nil {
			return nil, fmt.Errorf("create default config %q: %w", path, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("stat config %q: %w", path, err)
	}

	return reticulumconfig.LoadConfig(path)
}

func applyPageserverLogLevel(cfg *common.ReticulumConfig) {
	switch {
	case flagWasSet("log-level") && pageserverLogLevel >= debug.DebugCritical:
		debug.SetDebugLevel(pageserverLogLevel)
	case flagWasSet("log-level") && pageserverLogLevel != -1 && pageserverLogLevel < debug.DebugCritical:
		debug.SetDebugLevel(debug.DebugCritical)
	case flagWasSet("debug"):
	default:
		if cfg != nil {
			l := cfg.LogLevel
			if l >= debug.DebugCritical && l <= debug.DebugAll {
				debug.SetDebugLevel(l)
				return
			}
		}
		debug.SetDebugLevel(debug.DebugCritical)
	}
}
