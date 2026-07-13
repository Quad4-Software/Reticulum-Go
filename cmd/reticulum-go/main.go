// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"quad4/reticulum-go/internal/config"
	"quad4/reticulum-go/pkg/cli"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/controlapi"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/node"
	"quad4/reticulum-go/pkg/sandbox"
)

type Reticulum struct {
	*node.Node
	config            *common.ReticulumConfig
	controlAPI        *controlapi.Server
	announceHistory   map[string]announceRecord
	announceHistoryMu sync.RWMutex
}

type announceRecord struct {
	timestamp int64
	appData   []byte
}

func NewReticulum(cfg *common.ReticulumConfig) (*Reticulum, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	if err := initializeDirectories(); err != nil {
		return nil, fmt.Errorf("failed to initialize directories: %v", err)
	}
	debug.Log(debug.DebugInfo, "Directories initialized")

	n, err := node.New(cfg)
	if err != nil {
		return nil, err
	}

	r := &Reticulum{
		Node:            n,
		config:          cfg,
		announceHistory: make(map[string]announceRecord),
	}

	for name, ifaceConfig := range cfg.Interfaces {
		if !ifaceConfig.Enabled {
			continue
		}
		debug.Log(debug.DebugError, "Configuring interface", "name", name, "type", ifaceConfig.Type)
		debug.Log(debug.DebugInfo, "Interface configured", "name", name)
	}

	return r, nil
}

func main() {
	os.Exit(cli.Main(os.Args[1:], cli.Options{
		Argv0:       os.Args[0],
		VersionLine: versionLine(),
		RunDaemon:   runDaemonCLI,
	}))
}

func runDaemonCLI(args []string) int {
	opts, run, code := parseDaemonFlags(args)
	if !run {
		return code
	}
	return runDaemon(opts)
}

func runDaemon(opts daemonOptions) int {
	debug.Init()

	var cfg *common.ReticulumConfig
	var err error
	if opts.ConfigPath != "" {
		cfg, err = loadDaemonConfig(opts.ConfigPath)
	} else {
		cfg, err = config.InitConfig()
	}
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to initialize config", "error", err)
		return 1
	}

	applyDaemonLogging(cfg, opts)
	debug.Log(debug.DebugCritical, "Initializing Reticulum", "debug_level", debug.GetDebugLevel())
	debug.Log(debug.DebugInfo, "Configuration loaded", "path", cfg.ConfigPath)

	r, err := NewReticulum(cfg)
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to create Reticulum instance", "error", err)
		return 1
	}

	go r.monitorInterfaces()

	handler := NewAnnounceHandler(r, []string{"*"})
	r.Transport().RegisterAnnounceHandler(handler)

	if err := r.Start(); err != nil {
		debug.Log(debug.DebugCritical, "Failed to start Reticulum", "error", err)
		return 1
	}

	if err := sandbox.Apply(cfg); err != nil {
		debug.Log(debug.DebugCritical, "Sandbox application failed", "error", err)
		if cfg != nil && cfg.PanicOnInterfaceErr {
			return 1
		}
	}

	r.StartControlAPI()

	if runtime.GOOS != "windows" {
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
					applyDaemonLogging(newCfg, daemonOptions{DebugLevel: -1, JSONLogs: opts.JSONLogs})
					debug.Log(debug.DebugInfo, "Reloaded interfaces from config", "path", path)
				}
			}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	sigs := []os.Signal{os.Interrupt}
	if runtime.GOOS != "windows" {
		sigs = append(sigs, syscall.SIGTERM)
	}
	signal.Notify(sigChan, sigs...)
	<-sigChan

	debug.Log(debug.DebugCritical, "Shutting down...")
	if err := r.StopDaemon(); err != nil {
		debug.Log(debug.DebugCritical, "Error during shutdown", "error", err)
		return 1
	}
	debug.Log(debug.DebugCritical, "Goodbye!")
	return 0
}

func loadDaemonConfig(override string) (*common.ReticulumConfig, error) {
	path := override
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		path = filepath.Join(path, "config")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := config.CreateDefaultConfig(path); err != nil {
			return nil, err
		}
	}
	return config.LoadConfig(path)
}

func applyDaemonLogging(cfg *common.ReticulumConfig, opts daemonOptions) {
	if cfg == nil {
		return
	}
	level := cfg.LogLevel
	if opts.DebugLevel > 0 {
		level = opts.DebugLevel
	}
	if level > 0 {
		debug.SetDebugLevel(level)
	}
	if opts.JSONLogs || strings.EqualFold(cfg.LogFormat, "json") {
		debug.SetJSONFormat(true)
	}
	_ = debug.ConfigureDestination(cfg)
}

func (r *Reticulum) monitorInterfaces() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		for _, iface := range r.Interfaces() {
			if tcpClient, ok := iface.(*interfaces.TCPClientInterface); ok {
				stats := fmt.Sprintf("Interface %s status - Connected: %v, TX: %d bytes (%.2f Kbps), RX: %d bytes (%.2f Kbps)",
					iface.GetName(),
					tcpClient.IsConnected(),
					tcpClient.GetTxBytes(),
					float64(tcpClient.GetTxBytes()*8)/(5*1024),
					tcpClient.GetRxBytes(),
					float64(tcpClient.GetRxBytes()*8)/(5*1024),
				)

				if runtime.GOOS != "windows" {
					stats = fmt.Sprintf("%s, RTT: %v", stats, tcpClient.GetRTT())
				}

				debug.Log(debug.DebugVerbose, "Interface status", "stats", stats)
			}
		}
	}
}

func (r *Reticulum) StartControlAPI() {
	if !r.config.EnableControlAPI {
		return
	}
	api, err := controlapi.New(r.Transport(), r.Node, r.config)
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to initialize control API", "error", err)
		return
	}
	r.controlAPI = api
	go func() {
		if err := api.Serve(); err != nil {
			debug.Log(debug.DebugCritical, "Control API stopped", "error", err)
		}
	}()
}

func (r *Reticulum) StopDaemon() error {
	if r.controlAPI != nil {
		if err := r.controlAPI.Close(); err != nil {
			debug.Log(debug.DebugCritical, "Error closing control API", "error", err)
		}
		r.controlAPI = nil
	}
	return r.Node.Stop()
}

func initializeDirectories() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}

	basePath := filepath.Join(homeDir, ".reticulum-go")
	dirs := []string{
		basePath,
		filepath.Join(basePath, "storage"),
		filepath.Join(basePath, "storage", "destinations"),
		filepath.Join(basePath, "storage", "identities"),
		filepath.Join(basePath, "storage", "ratchets"),
		filepath.Join(basePath, "storage", "cache"),
		filepath.Join(basePath, "storage", "cache", "announces"),
		filepath.Join(basePath, "storage", "resources"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil { // #nosec G301
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}
	return nil
}

type AnnounceHandler struct {
	aspectFilter []string
	reticulum    *Reticulum
}

func NewAnnounceHandler(r *Reticulum, aspectFilter []string) *AnnounceHandler {
	return &AnnounceHandler{
		aspectFilter: aspectFilter,
		reticulum:    r,
	}
}

func (h *AnnounceHandler) AspectFilter() []string {
	return h.aspectFilter
}

func (h *AnnounceHandler) ReceivedAnnounce(destHash []byte, id any, appData []byte, hops uint8) error {
	debug.Log(debug.DebugInfo, "Received announce", "hash", fmt.Sprintf("%x", destHash), "appData_len", len(appData), "hops", hops)
	debug.Log(debug.DebugPackets, "Announce appData", "data", fmt.Sprintf("%x", appData))

	if annID, ok := id.(*identity.Identity); ok {
		debug.Log(debug.DebugAll, "Announce identity", "hash", annID.GetHexHash(), "pubkey", fmt.Sprintf("%x", annID.GetPublicKey()))

		h.reticulum.announceHistoryMu.Lock()
		h.reticulum.announceHistory[annID.GetHexHash()] = announceRecord{
			timestamp: time.Now().Unix(),
			appData:   appData,
		}
		h.reticulum.announceHistoryMu.Unlock()
	}

	return nil
}

func (h *AnnounceHandler) ReceivePathResponses() bool {
	return true
}
