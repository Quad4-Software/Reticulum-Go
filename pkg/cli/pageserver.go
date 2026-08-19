// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

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
	"quad4/reticulum-go/pkg/pageserver"
	"quad4/reticulum-go/pkg/reticulumconfig"
)

// RunPageserver serves NomadNet-style pages and files over Reticulum.
func RunPageserver(args []string, opt ...Options) int {
	_, stderr := cliIO(opt)
	fs := flag.NewFlagSet("pageserver", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		configPath          string
		pagesDir            string
		filesDir            string
		nodeName            string
		announceIntervalMin int
		pageRefreshSec      int
		fileRefreshSec      int
		logLevel            int
		identityOverride    string
		disablePageStats    bool
	)

	fs.StringVar(&configPath, "config", "", "Reticulum config file (default ~/.reticulum-go/config)")
	fs.StringVar(&configPath, "c", "", "same as -config")
	fs.StringVar(&pagesDir, "pages-dir", "pages", "pages directory for /page/")
	fs.StringVar(&pagesDir, "p", "pages", "same as -pages-dir")
	fs.StringVar(&filesDir, "files-dir", "files", "files directory for /file/")
	fs.StringVar(&filesDir, "f", "files", "same as -files-dir")
	fs.StringVar(&nodeName, "node-name", pageserver.AppName, "node display name in announces")
	fs.StringVar(&nodeName, "n", pageserver.AppName, "same as -node-name")
	fs.IntVar(&announceIntervalMin, "announce-interval", 360, "periodic announces every N minutes (0 = once)")
	fs.IntVar(&announceIntervalMin, "a", 360, "same as -announce-interval")
	fs.IntVar(&pageRefreshSec, "page-refresh", 0, "rescan pages every N seconds (0 = startup only)")
	fs.IntVar(&pageRefreshSec, "pages-refresh-interval", 0, "same as -page-refresh")
	fs.IntVar(&fileRefreshSec, "file-refresh", 0, "rescan files every N seconds (0 = startup only)")
	fs.IntVar(&fileRefreshSec, "files-refresh-interval", 0, "same as -file-refresh")
	fs.IntVar(&logLevel, "log-level", -1, "log verbosity 0-7 (-1 = from config)")
	fs.StringVar(&identityOverride, "identity", "", "identity file path")
	fs.StringVar(&identityOverride, "identity-path", "", "same as -identity")
	fs.BoolVar(&disablePageStats, "no-page-stats", false, "disable built-in page view stats")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := loadOrInitConfig(configPath)
	if err != nil {
		applyPageserverLogLevel(fs, logLevel, nil)
		debug.Init()
		debug.GetLogger().Error("Failed to load configuration", "error", err)
		return 1
	}
	applyPageserverLogLevel(fs, logLevel, cfg)
	debug.Init()

	opts := pageserver.Options{
		PagesDir:                pagesDir,
		FilesDir:                filesDir,
		PageRefreshInterval:     time.Duration(pageRefreshSec) * time.Second,
		FileRefreshInterval:     time.Duration(fileRefreshSec) * time.Second,
		AnnounceIntervalMinutes: announceIntervalMin,
		IdentityFileOverride:    identityOverride,
		NodeDisplayName:         pageserver.ClampDisplayName(nodeName, pageserver.AppName),
		DisablePageStats:        disablePageStats,
	}

	r, err := pageserver.NewReticulum(cfg, opts)
	if err != nil {
		debug.GetLogger().Error("Failed to create Reticulum instance", "error", err)
		return 1
	}

	go r.MonitorInterfaces()

	handler := pageserver.NewAnnounceHandler(r, []string{"*"})
	r.Transport().RegisterAnnounceHandler(handler)

	if err := r.Start(); err != nil {
		debug.GetLogger().Error("Failed to start Reticulum", "error", err)
		return 1
	}

	pageserver.PrintStartupSummary(r)

	sigChan := make(chan os.Signal, 1)
	sigs := []os.Signal{os.Interrupt}
	if runtime.GOOS != "windows" {
		sigs = append(sigs, syscall.SIGTERM)
	}
	signal.Notify(sigChan, sigs...)
	<-sigChan

	debug.Log(debug.DebugInfo, "Shutting down...")
	if err := r.Stop(); err != nil {
		debug.Log(debug.DebugError, "Error during shutdown", "error", err)
	}
	debug.Log(debug.DebugInfo, "Goodbye!")
	return 0
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

func applyPageserverLogLevel(fs *flag.FlagSet, pageserverLogLevel int, cfg *common.ReticulumConfig) {
	logLevelSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "log-level" {
			logLevelSet = true
		}
	})
	if logLevelSet {
		debug.SetDebugLevel(pageserverLogLevel)
		return
	}
	if cfg != nil {
		debug.SetDebugLevel(cfg.LogLevel)
		return
	}
	debug.SetDebugLevel(debug.DebugInfo)
}
