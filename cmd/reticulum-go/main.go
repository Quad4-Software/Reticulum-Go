// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/internal/config"
	"github.com/Quad4-Software/Reticulum-Go/pkg/buffer"
	"github.com/Quad4-Software/Reticulum-Go/pkg/channel"
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/sandbox"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

type Reticulum struct {
	config            *common.ReticulumConfig
	transport         *transport.Transport
	interfaces        []interfaces.Interface
	channels          map[string]*channel.Channel
	buffers           map[string]*buffer.Buffer
	pathRequests      map[string]*common.PathRequest
	announceHistory   map[string]announceRecord
	announceHistoryMu sync.RWMutex
	reloadMu          sync.Mutex
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

	t := transport.NewTransport(cfg)
	debug.Log(debug.DebugInfo, "Transport initialized")

	r := &Reticulum{
		config:          cfg,
		transport:       t,
		interfaces:      make([]interfaces.Interface, 0),
		channels:        make(map[string]*channel.Channel),
		buffers:         make(map[string]*buffer.Buffer),
		pathRequests:    make(map[string]*common.PathRequest),
		announceHistory: make(map[string]announceRecord),
	}

	// Initialize interfaces from config
	for name, ifaceConfig := range cfg.Interfaces {
		if !ifaceConfig.Enabled {
			continue
		}

		iface, err := interfaces.NewFromConfig(name, ifaceConfig)
		if err != nil {
			if cfg.PanicOnInterfaceErr {
				return nil, fmt.Errorf("failed to create interface %s: %v", name, err)
			}
			debug.Log(debug.DebugCritical, "Error creating interface", "name", name, "error", err)
			continue
		}

		debug.Log(debug.DebugError, "Configuring interface", "name", name, "type", ifaceConfig.Type)
		r.interfaces = append(r.interfaces, iface)
		debug.Log(debug.DebugInfo, "Interface configured", "name", name)
	}

	return r, nil
}

func (r *Reticulum) handleInterface(iface common.NetworkInterface) {
	debug.Log(debug.DebugInfo, "Setting up interface", "name", iface.GetName(), "type", fmt.Sprintf("%T", iface))

	ch := channel.NewChannel(&transportWrapper{r.transport})
	r.channels[iface.GetName()] = ch

	rw := buffer.CreateBidirectionalBuffer(
		1,
		2,
		ch,
		func(size int) {
			data := make([]byte, size)
			debug.Log(debug.DebugPackets, "Interface reading bytes from buffer", "name", iface.GetName(), "size", size)
			iface.ProcessIncoming(data)

			if len(data) > 0 {
				debug.Log(debug.DebugTrace, "Interface received packet type", "name", iface.GetName(), "type", fmt.Sprintf("0x%02x", data[0]))
				r.transport.HandlePacket(data, iface)
			}
		},
	)

	r.buffers[iface.GetName()] = &buffer.Buffer{
		ReadWriter: rw,
	}
}

func (r *Reticulum) monitorInterfaces() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		for _, iface := range r.interfaces {
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

func main() {
	debug.Init()
	debug.Log(debug.DebugCritical, "Initializing Reticulum", "debug_level", debug.GetDebugLevel())

	cfg, err := config.InitConfig()
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to initialize config", "error", err)
		os.Exit(1)
	}
	debug.Log(debug.DebugInfo, "Configuration loaded", "path", cfg.ConfigPath)

	r, err := NewReticulum(cfg)
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to create Reticulum instance", "error", err)
		os.Exit(1)
	}

	// Start monitoring interfaces
	go r.monitorInterfaces()

	// Register announce handler
	handler := NewAnnounceHandler(r, []string{"*"})
	r.transport.RegisterAnnounceHandler(handler)

	// Start Reticulum
	if err := r.Start(); err != nil {
		debug.Log(debug.DebugCritical, "Failed to start Reticulum", "error", err)
		os.Exit(1)
	}

	// Apply sandbox after all privileged initialization is complete.
	if err := sandbox.Apply(cfg); err != nil {
		debug.Log(debug.DebugCritical, "Sandbox application failed", "error", err)
		if cfg != nil && cfg.PanicOnInterfaceErr {
			os.Exit(1)
		}
	}

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
	if err := r.Stop(); err != nil {
		debug.Log(debug.DebugCritical, "Error during shutdown", "error", err)
	}
	debug.Log(debug.DebugCritical, "Goodbye!")
}

type transportWrapper struct {
	*transport.Transport
}

func (tw *transportWrapper) GetRTT() float64 {
	return 0.1
}

func (tw *transportWrapper) RTT() float64 {
	return tw.GetRTT()
}

func (tw *transportWrapper) GetStatus() byte {
	return transport.StatusActive
}

func (tw *transportWrapper) Send(data []byte) any {
	p := &packet.Packet{
		PacketType: packet.PacketTypeData,
		Hops:       0,
		Data:       data,
		HeaderType: packet.HeaderType1,
	}

	err := tw.Transport.SendPacket(p)
	if err != nil {
		return nil
	}
	return p
}

func (tw *transportWrapper) Resend(p any) error {
	if pkt, ok := p.(*packet.Packet); ok {
		return tw.Transport.SendPacket(pkt)
	}
	return fmt.Errorf("invalid packet type")
}

func (tw *transportWrapper) SetPacketTimeout(packet any, callback func(any), timeout time.Duration) {
	time.AfterFunc(timeout, func() {
		callback(packet)
	})
}

func (tw *transportWrapper) SetPacketDelivered(packet any, callback func(any)) {
	callback(packet)
}

func (tw *transportWrapper) GetLinkID() []byte {
	return nil
}

func (tw *transportWrapper) HandleInbound(pkt *packet.Packet) error {
	return nil
}

func (tw *transportWrapper) ValidateLinkProof(pkt *packet.Packet, networkIface common.NetworkInterface) error {
	return nil
}

func (tw *transportWrapper) LinkedNetworkInterface() common.NetworkInterface {
	return nil
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

func (r *Reticulum) Start() error {
	debug.Log(debug.DebugError, "Starting Reticulum...")

	if err := r.transport.Start(); err != nil {
		return fmt.Errorf("failed to start transport: %v", err)
	}
	debug.Log(debug.DebugInfo, "Transport started successfully")

	type interfaceStartResult struct {
		iface interfaces.Interface
		err   error
	}

	// Start interfaces in parallel so one slow dial/listen does not
	// delay the whole node startup.
	results := make(chan interfaceStartResult, len(r.interfaces))
	for _, iface := range r.interfaces {
		go func() {
			debug.Log(debug.DebugError, "Starting interface", "name", iface.GetName())
			results <- interfaceStartResult{iface: iface, err: iface.Start()}
		}()
	}

	started := make([]interfaces.Interface, 0, len(r.interfaces))
	for range len(r.interfaces) {
		res := <-results
		if res.err != nil {
			if r.config.PanicOnInterfaceErr {
				return fmt.Errorf("failed to start interface %s: %v", res.iface.GetName(), res.err)
			}
			debug.Log(debug.DebugCritical, "Error starting interface", "name", res.iface.GetName(), "error", res.err)
			continue
		}
		started = append(started, res.iface)
	}

	for _, iface := range started {
		netIface, ok := iface.(common.NetworkInterface)
		if !ok {
			continue
		}
		if err := r.transport.RegisterInterface(iface.GetName(), netIface); err != nil {
			debug.Log(debug.DebugCritical, "Failed to register interface with transport", "name", iface.GetName(), "error", err)
			continue
		}
		debug.Log(debug.DebugInfo, "Registered interface with transport", "name", iface.GetName())
		r.handleInterface(netIface)
		debug.Log(debug.DebugInfo, "Interface started successfully", "name", iface.GetName())
	}

	if !r.hasOnlineInterface() {
		debug.Log(debug.DebugInfo, "No interface online yet; continuing startup and waiting for dynamic bring-up")
	}

	debug.Log(debug.DebugError, "Reticulum started successfully")
	return nil
}

func (r *Reticulum) hasOnlineInterface() bool {
	for _, iface := range r.interfaces {
		if iface != nil && iface.IsOnline() && iface.IsEnabled() {
			return true
		}
	}
	return false
}

func (r *Reticulum) Stop() error {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	debug.Log(debug.DebugError, "Stopping Reticulum...")

	for _, buf := range r.buffers {
		if err := buf.Close(); err != nil {
			debug.Log(debug.DebugCritical, "Error closing buffer", "error", err)
		}
	}

	for _, ch := range r.channels {
		if err := ch.Close(); err != nil {
			debug.Log(debug.DebugCritical, "Error closing channel", "error", err)
		}
	}

	for _, iface := range r.interfaces {
		if err := iface.Stop(); err != nil {
			debug.Log(debug.DebugCritical, "Error stopping interface", "name", iface.GetName(), "error", err)
		}
	}

	if err := r.transport.Close(); err != nil {
		return fmt.Errorf("failed to close transport: %v", err)
	}

	debug.Log(debug.DebugError, "Reticulum stopped successfully")
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
