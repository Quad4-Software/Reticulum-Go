// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"git.quad4.io/Go-Libs/msgpack/v5/pkg/msgpack"

	"git.quad4.io/Networks/Reticulum-Go/pkg/buffer"
	"git.quad4.io/Networks/Reticulum-Go/pkg/channel"
	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/destination"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
	"git.quad4.io/Networks/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/Networks/Reticulum-Go/pkg/link"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
	"git.quad4.io/Networks/Reticulum-Go/pkg/reticulumconfig"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

var (
	interceptPackets     = flag.Bool("intercept-packets", false, "Enable packet interception")
	interceptOutput      = flag.String("intercept-output", "packets.log", "Output file for intercepted packets")
	logLevel             = flag.Int("log-level", -1, "Log level override (1-7). Alias for -debug")
	configPath           = flag.String("config", "", "Path to Reticulum config file (default: ~/.reticulum-go/config). Created with default interfaces if missing.")
	useUDP               = flag.Bool("udp", false, "Add a local UDP interface overlay on top of the loaded config")
	listenPort           = flag.Int("listen-port", 4242, "UDP listen port when -udp is enabled")
	targetPort           = flag.Int("target-port", 0, "UDP target port when -udp is enabled (0 disables target)")
	freshIdentity        = flag.Bool("fresh-identity", false, "Remove persisted identity before startup to force a new destination hash")
	identityPath         = flag.String("identity-path", "", "Override identity file path (defaults to ~/.reticulum-go/storage/identity)")
	noAutoDiscovery      = flag.Bool("no-auto-discovery", false, "Disable the AutoInterface (UDP discovery), forcing TCP-only operation")
	announceInterval     = flag.Duration("announce-interval", defaultAnnounceInterval, "Time between periodic destination announces after the initial boot announce. Set to 0 or negative to disable periodic announces.")
	pagesDirFlag         = flag.String("pages-dir", "pages", "Directory of .mu pages served under /page/")
	filesDirFlag         = flag.String("files-dir", "files", "Directory of files served under /file/")
	pagesRefreshInterval = flag.Duration("pages-refresh-interval", 0, "Rescan pages-dir and update /page/ handlers on this interval (0 disables). New and removed files apply without restarting.")
	filesRefreshInterval = flag.Duration("files-refresh-interval", 0, "Rescan files-dir and update /file/ handlers on this interval (0 disables). New and removed files apply without restarting.")
	nodeNameFlag         = flag.String("node-name", APP_NAME, "Node name advertised in announces as NomadNetwork-style app_data (max 255 bytes).")
)

const maxNodeNameBytes = 255

const (
	ANNOUNCE_RATE_TARGET  = 3600
	ANNOUNCE_RATE_GRACE   = 3
	ANNOUNCE_RATE_PENALTY = 7200
	MAX_ANNOUNCE_HOPS     = 128
	APP_NAME              = "Reticulum-Go Test Node"
	APP_ASPECT            = "node"

	defaultAnnounceInterval = 6 * time.Hour

	minAnnounceInterval = 1 * time.Minute
)

// Reticulum owns the transport, destination, static /page and /file handlers, and announce history.
type Reticulum struct {
	config            *common.ReticulumConfig
	transport         *transport.Transport
	interfaces        []interfaces.Interface
	channels          map[string]*channel.Channel
	buffers           map[string]*buffer.Buffer
	pathRequests      map[string]*common.PathRequest
	announceHistory   map[string]announceRecord
	announceHistoryMu sync.RWMutex
	identity          *identity.Identity
	destination       *destination.Destination

	maxTransferSize int16
	nodeEnabled     bool
	nodeTimestamp   int64

	pagesPath        string
	filesPath        string
	establishedLinks map[string]*link.Link
	linksMutex       sync.RWMutex

	pagesRefreshInterval time.Duration
	filesRefreshInterval time.Duration
	registeredPagePaths  map[string]struct{}
	registeredFilePaths  map[string]struct{}
	staticPagesMu        sync.Mutex
	staticFilesMu        sync.Mutex
	refreshStop          chan struct{}
	refreshOnce          sync.Once
}

type announceRecord struct {
	timestamp int64
	appData   []byte
}

// resolveNodeName returns the announced node name, falling back to APP_NAME
// when the flag is unset or whitespace-only and clamping to maxNodeNameBytes
// so the value can always fit inside an msgpack bin8 announce field.
func resolveNodeName() string {
	name := strings.TrimSpace(*nodeNameFlag)
	if name == "" {
		name = APP_NAME
	}
	if len(name) > maxNodeNameBytes {
		debug.Log(debug.DebugCritical,
			"node name exceeds max length, truncating",
			"max_bytes", maxNodeNameBytes,
			"got_bytes", len(name),
		)
		name = name[:maxNodeNameBytes]
	}
	return name
}

// NewReticulum loads or creates identity, opens the destination, wires link and static handlers, and builds interfaces from cfg.
func NewReticulum(cfg *common.ReticulumConfig) (*Reticulum, error) {
	if cfg == nil {
		cfg = common.DefaultConfig()
	}

	cfg.AppName = resolveNodeName()
	if cfg.AppAspect == "" {
		cfg.AppAspect = APP_ASPECT
	}

	if err := initializeDirectories(); err != nil {
		return nil, fmt.Errorf("failed to initialize directories: %v", err)
	}
	debug.Log(debug.DebugInfo, "Directories initialized")

	t := transport.NewTransport(cfg)
	debug.Log(debug.DebugInfo, "Transport initialized")

	identityPath := getIdentityPath()
	if *freshIdentity {
		if err := os.Remove(identityPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove identity at %s: %v", identityPath, err)
		}
		debug.Log(debug.DebugInfo, "Forced fresh identity by removing persisted identity file", "path", identityPath)
	}

	var ident *identity.Identity

	if _, err := os.Stat(identityPath); err == nil {
		ident, err = identity.LoadIdentityFile(identityPath, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to load identity: %v", err)
		}
		debug.Log(debug.DebugError, "Loaded existing identity", "hash", fmt.Sprintf("%x", ident.Hash()))
	} else {
		ident, err = identity.NewIdentity()
		if err != nil {
			return nil, fmt.Errorf("failed to create identity: %v", err)
		}
		debug.Log(debug.DebugError, "Created new identity", "hash", fmt.Sprintf("%x", ident.Hash()))

		if err := ident.ToFile(identityPath); err != nil {
			debug.Log(debug.DebugError, "Failed to save identity to file", "error", err)
		} else {
			debug.Log(debug.DebugInfo, "Identity saved to file", "path", identityPath)
		}
	}

	t.SetIdentity(ident)

	debug.Log(debug.DebugInfo, "Creating destination...")
	dest, err := destination.New(
		ident,
		destination.In,
		destination.Single,
		"nomadnetwork",
		t,
		"node",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination: %v", err)
	}
	debug.Log(debug.DebugInfo, "Created destination with hash", "hash", fmt.Sprintf("%x", dest.GetHash()))

	nodeTimestamp := time.Now().Unix()

	r := &Reticulum{
		config:          cfg,
		transport:       t,
		interfaces:      make([]interfaces.Interface, 0),
		channels:        make(map[string]*channel.Channel),
		buffers:         make(map[string]*buffer.Buffer),
		pathRequests:    make(map[string]*common.PathRequest),
		announceHistory: make(map[string]announceRecord),
		identity:        ident,
		destination:     dest,

		maxTransferSize: 500,
		nodeEnabled:     true,
		nodeTimestamp:   nodeTimestamp,

		pagesPath:            *pagesDirFlag,
		filesPath:            *filesDirFlag,
		establishedLinks:     make(map[string]*link.Link),
		pagesRefreshInterval: *pagesRefreshInterval,
		filesRefreshInterval: *filesRefreshInterval,
		refreshStop:          make(chan struct{}),
	}

	dest.AcceptsLinks(true)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	ratchetPath := filepath.Join(homeDir, ".reticulum-go", "storage", "ratchets", r.identity.GetHexHash())
	dest.EnableRatchets(ratchetPath)
	dest.SetProofStrategy(destination.ProveApp)

	dest.SetLinkEstablishedCallback(r.onLinkEstablished)

	debug.Log(debug.DebugVerbose, "Configured destination features")

	r.registerStaticHandlers()

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
	flag.Parse()
	cfg, err := loadOrInitConfig(*configPath)
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

	if *noAutoDiscovery {
		if iface, ok := cfg.Interfaces["Auto Discovery"]; ok {
			iface.Enabled = false
		}
	}

	if *useUDP {
		targetHost := ""
		if *targetPort > 0 {
			targetHost = fmt.Sprintf("127.0.0.1:%d", *targetPort)
		}
		cfg.Interfaces["UDP"] = &common.InterfaceConfig{
			Type:       "UDPInterface",
			Enabled:    true,
			Address:    fmt.Sprintf("0.0.0.0:%d", *listenPort),
			TargetHost: targetHost,
			Name:       "UDP",
		}
	}

	r, err := NewReticulum(cfg)
	if err != nil {
		debug.GetLogger().Error("Failed to create Reticulum instance", "error", err)
		os.Exit(1)
	}

	go r.monitorInterfaces()

	handler := NewAnnounceHandler(r, []string{"*"})
	r.transport.RegisterAnnounceHandler(handler)

	if err := r.Start(); err != nil {
		debug.GetLogger().Error("Failed to start Reticulum", "error", err)
		os.Exit(1)
	}

	printPageserverStartupSummary(r)

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

func (tw *transportWrapper) Send(data []byte) interface{} {
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

func (tw *transportWrapper) Resend(p interface{}) error {
	if pkt, ok := p.(*packet.Packet); ok {
		return tw.Transport.SendPacket(pkt)
	}
	return fmt.Errorf("invalid packet type")
}

func (tw *transportWrapper) SetPacketTimeout(packet interface{}, callback func(interface{}), timeout time.Duration) {
	time.AfterFunc(timeout, func() {
		callback(packet)
	})
}

func (tw *transportWrapper) SetPacketDelivered(packet interface{}, callback func(interface{})) {
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

func getIdentityPath() string {
	if *identityPath != "" {
		return *identityPath
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".reticulum-go", "storage", "identity")
	}
	return filepath.Join(homeDir, ".reticulum-go", "storage", "identity")
}

// Start boots the transport, registers interfaces, sends the initial announce, and starts optional refresh loops.
func (r *Reticulum) Start() error {
	debug.Log(debug.DebugError, "Starting Reticulum...")

	if err := r.transport.Start(); err != nil {
		return fmt.Errorf("failed to start transport: %v", err)
	}
	debug.Log(debug.DebugInfo, "Transport started successfully")

	if err := r.transport.InitializePathRequestHandler(); err != nil {
		debug.Log(debug.DebugError, "Failed to initialize path request handler", "error", err)
	}

	for _, iface := range r.interfaces {
		debug.Log(debug.DebugError, "Starting interface", "name", iface.GetName())
		if err := iface.Start(); err != nil {
			if r.config.PanicOnInterfaceErr {
				return fmt.Errorf("failed to start interface %s: %v", iface.GetName(), err)
			}
			debug.Log(debug.DebugCritical, "Error starting interface", "name", iface.GetName(), "error", err)
			continue
		}

		if netIface, ok := iface.(common.NetworkInterface); ok {
			if err := r.transport.RegisterInterface(iface.GetName(), netIface); err != nil {
				debug.Log(debug.DebugCritical, "Failed to register interface with transport", "name", iface.GetName(), "error", err)
			} else {
				debug.Log(debug.DebugInfo, "Registered interface with transport", "name", iface.GetName())
			}
			r.handleInterface(netIface)
		}
		debug.Log(debug.DebugInfo, "Interface started successfully", "name", iface.GetName())
	}

	time.Sleep(2 * time.Second)

	nodeName := r.config.AppName
	if nodeName == "" {
		nodeName = APP_NAME
	}
	debug.Log(debug.DebugInfo, "Sending initial announce",
		"dest_hash", fmt.Sprintf("%x", r.destination.GetHash()),
		"node_name", nodeName,
	)
	r.destination.SetDefaultAppData([]byte(nodeName))
	announceStartTime := time.Now()
	if err := r.destination.Announce(false, nil, nil); err != nil {
		debug.Log(debug.DebugCritical, "Failed to send initial announce", "error", err, "elapsed", time.Since(announceStartTime).Seconds())
	} else {
		debug.Log(debug.DebugInfo, "Initial announce sent successfully", "elapsed", time.Since(announceStartTime).Seconds())
	}

	interval := *announceInterval
	if interval <= 0 {
		debug.Log(debug.DebugInfo, "Periodic announces disabled (announce-interval <= 0); only the initial announce was sent")
	} else {
		if interval < minAnnounceInterval {
			debug.Log(debug.DebugCritical,
				"Configured announce-interval is below the minimum; clamping to avoid flooding peers",
				"requested", interval,
				"min", minAnnounceInterval,
			)
			interval = minAnnounceInterval
		}
		debug.Log(debug.DebugInfo, "Starting periodic announce loop", "interval", interval)
		go func(period time.Duration) {
			ticker := time.NewTicker(period)
			defer ticker.Stop()
			for range ticker.C {
				debug.Log(debug.DebugInfo, "Sending periodic announce", "interval", period)
				if err := r.destination.Announce(false, nil, nil); err != nil {
					debug.Log(debug.DebugCritical, "Could not send announce", "error", err)
				}
			}
		}(interval)
	}

	go r.monitorInterfaces()

	if r.pagesRefreshInterval > 0 {
		go r.staticPagesRefreshLoop()
	}
	if r.filesRefreshInterval > 0 {
		go r.staticFilesRefreshLoop()
	}

	debug.Log(debug.DebugError, "Reticulum started successfully")
	return nil
}

func (r *Reticulum) staticPagesRefreshLoop() {
	ticker := time.NewTicker(r.pagesRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.refreshStop:
			return
		case <-ticker.C:
			r.syncPageHandlers()
		}
	}
}

func (r *Reticulum) staticFilesRefreshLoop() {
	ticker := time.NewTicker(r.filesRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.refreshStop:
			return
		case <-ticker.C:
			r.syncFileHandlers()
		}
	}
}

func (r *Reticulum) Stop() error {
	debug.Log(debug.DebugError, "Stopping Reticulum...")

	r.refreshOnce.Do(func() { close(r.refreshStop) })

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

// AnnounceHandler records announces and decodes optional msgpack node metadata from app_data.
type AnnounceHandler struct {
	aspectFilter []string
	reticulum    *Reticulum
}

// NewAnnounceHandler builds a handler that accepts all aspects when aspectFilter is ["*"].
func NewAnnounceHandler(r *Reticulum, aspectFilter []string) *AnnounceHandler {
	return &AnnounceHandler{
		aspectFilter: aspectFilter,
		reticulum:    r,
	}
}

func (h *AnnounceHandler) AspectFilter() []string {
	return h.aspectFilter
}

func (h *AnnounceHandler) ReceivedAnnounce(destHash []byte, id interface{}, appData []byte, hops uint8) error {
	debug.Log(debug.DebugInfo, "Received announce", "hash", fmt.Sprintf("%x", destHash))
	debug.Log(debug.DebugPackets, "Raw announce data", "data", fmt.Sprintf("%x", appData))
	debug.Log(debug.DebugInfo, "MAIN HANDLER: Received announce", "hash", fmt.Sprintf("%x", destHash), "appData_len", len(appData))

	var isNode bool
	var nodeEnabled bool
	var nodeTimestamp int64
	var nodeMaxSize int16

	if len(appData) > 0 {
		var decoded interface{}
		if err := msgpack.Unmarshal(appData, &decoded); err == nil {
			if enabled, ts, maxSize, ok := parseNodeStatusAppData(decoded); ok {
				isNode = true
				nodeEnabled = enabled
				nodeTimestamp = ts
				nodeMaxSize = maxSize
				debug.Log(
					debug.DebugInfo,
					"Parsed node status appData",
					"enabled", nodeEnabled,
					"timestamp", nodeTimestamp,
					"maxsize_kb", nodeMaxSize,
				)
			}

			if nodeName, ok := extractNodeName(decoded); ok {
				debug.Log(debug.DebugInfo, "Parsed node name", "name", nodeName)
				debug.Log(debug.DebugInfo, "Announced node", "name", nodeName)
			} else if !isNode {
				debug.Log(debug.DebugInfo, "Parsed structured appData", "value", fmt.Sprintf("%v", decoded))
			}
		} else {
			if utf8.Valid(appData) {
				nodeName := strings.TrimSpace(string(appData))
				if nodeName != "" {
					debug.Log(debug.DebugInfo, "Raw node name", "name", nodeName)
					debug.Log(debug.DebugInfo, "Announced node", "name", nodeName)
				} else {
					debug.Log(debug.DebugInfo, "Raw appData is empty text after trim")
				}
			} else {
				debug.Log(debug.DebugInfo, "Binary appData (non-text)", "hex", fmt.Sprintf("%x", appData))
			}
		}
	} else {
		debug.Log(debug.DebugInfo, "No appData (empty announce)")
	}

	if identity, ok := id.(*identity.Identity); ok {
		debug.Log(debug.DebugAll, "Identity details")
		debug.Log(debug.DebugAll, "Identity hash", "hash", identity.GetHexHash())
		debug.Log(debug.DebugAll, "Identity public key", "key", fmt.Sprintf("%x", identity.GetPublicKey()))

		ratchets := identity.GetRatchets()
		debug.Log(debug.DebugAll, "Active ratchets", "count", len(ratchets))

		if len(ratchets) > 0 {
			ratchetKey := identity.GetCurrentRatchetKey()
			if ratchetKey != nil {
				ratchetID := identity.GetRatchetID(ratchetKey)
				debug.Log(debug.DebugAll, "Current ratchet ID", "id", fmt.Sprintf("%x", ratchetID))
			}
		}

		recordType := "peer"
		if isNode {
			recordType = "node"
			debug.Log(debug.DebugInfo, "Storing node in announce history", "enabled", nodeEnabled, "timestamp", nodeTimestamp, "maxsize", fmt.Sprintf("%dKB", nodeMaxSize))
		}

		h.reticulum.announceHistoryMu.Lock()
		h.reticulum.announceHistory[identity.GetHexHash()] = announceRecord{
			timestamp: time.Now().Unix(),
			appData:   appData,
		}
		h.reticulum.announceHistoryMu.Unlock()

		debug.Log(debug.DebugVerbose, "Stored announce in history", "type", recordType, "identity", identity.GetHexHash())
	}

	return nil
}

func (h *AnnounceHandler) ReceivePathResponses() bool {
	return true
}

func extractNodeName(decoded interface{}) (string, bool) {
	if decoded == nil {
		return "", false
	}

	if str, ok := msgpackString(decoded); ok {
		str = strings.TrimSpace(str)
		return str, str != ""
	}

	arr, ok := decoded.([]interface{})
	if !ok || len(arr) == 0 {
		return "", false
	}

	for _, v := range arr {
		if str, ok := msgpackString(v); ok {
			str = strings.TrimSpace(str)
			if str != "" {
				return str, true
			}
		}
	}

	return "", false
}

func parseNodeStatusAppData(decoded interface{}) (enabled bool, timestamp int64, maxSizeKB int16, ok bool) {
	arr, ok := decoded.([]interface{})
	if !ok || len(arr) < 3 {
		return false, 0, 0, false
	}

	enabledVal, ok := arr[0].(bool)
	if !ok {
		return false, 0, 0, false
	}

	timestampVal, ok := msgpackInt64(arr[1])
	if !ok {
		return false, 0, 0, false
	}

	maxSizeVal, ok := msgpackInt64(arr[2])
	if !ok {
		return false, 0, 0, false
	}

	return enabledVal, timestampVal, int16(maxSizeVal), true // #nosec G115
}

func msgpackString(v interface{}) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	case []byte:
		if utf8.Valid(val) {
			return string(val), true
		}
		return "", false
	default:
		return "", false
	}
}

func msgpackInt64(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case int:
		return int64(val), true
	case int8:
		return int64(val), true
	case int16:
		return int64(val), true
	case int32:
		return int64(val), true
	case int64:
		return val, true
	case uint:
		return int64(val), true // #nosec G115
	case uint8:
		return int64(val), true
	case uint16:
		return int64(val), true
	case uint32:
		return int64(val), true
	case uint64:
		return int64(val), true // #nosec G115
	default:
		return 0, false
	}
}

func (r *Reticulum) GetDestination() *destination.Destination {
	return r.destination
}

// createNodeAppData encodes msgpack [enabled, timestamp_unix, max_kb] for structured node announces.
func (r *Reticulum) createNodeAppData() []byte {
	appData := []byte{0x93}

	if r.nodeEnabled {
		appData = append(appData, 0xc3)
	} else {
		appData = append(appData, 0xc2)
	}

	r.nodeTimestamp = time.Now().Unix()
	appData = append(appData, 0xd2)
	timeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(timeBytes, uint32(r.nodeTimestamp)) // #nosec G115
	appData = append(appData, timeBytes...)

	appData = append(appData, 0xd1)
	sizeBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(sizeBytes, uint16(r.maxTransferSize)) // #nosec G115
	appData = append(appData, sizeBytes...)

	debug.Log(debug.DebugAll, "Created node appData", "enable", r.nodeEnabled, "timestamp", r.nodeTimestamp, "maxsize", r.maxTransferSize, "data", fmt.Sprintf("%x", appData))
	return appData
}

func (r *Reticulum) onLinkEstablished(linkInterface interface{}) {
	startTime := time.Now()
	debug.Log(debug.DebugInfo, "Link established callback called", "interface_type", fmt.Sprintf("%T", linkInterface))

	l, ok := linkInterface.(*link.Link)
	if !ok {
		debug.Log(debug.DebugError, "Invalid link interface type, expected *link.Link")
		return
	}

	r.linksMutex.Lock()
	linkIDStr := fmt.Sprintf("%x", l.GetLinkID())
	r.establishedLinks[linkIDStr] = l
	r.linksMutex.Unlock()

	debug.Log(debug.DebugInfo, "Link established successfully", "link_id", linkIDStr, "rtt", l.GetRTT(), "elapsed", time.Since(startTime).Seconds())

	l.SetPacketCallback(func(data []byte, pkt *packet.Packet) {
		r.handleLinkPacket(l, data, pkt)
	})
}

func (r *Reticulum) registerStaticHandlers() {
	r.syncPageHandlers()
	r.syncFileHandlers()
	debug.Log(debug.DebugInfo, "Static page and file handlers registered")
}

func (r *Reticulum) syncPageHandlers() {
	r.staticPagesMu.Lock()
	defer r.staticPagesMu.Unlock()

	pagesDir := r.pagesPath
	if err := os.MkdirAll(pagesDir, 0750); err != nil { // #nosec G301
		debug.Log(debug.DebugError, "Failed to create pages directory", "error", err)
		return
	}

	current := make(map[string]struct{})
	walkErr := filepath.Walk(pagesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(pagesDir, path)
		if err != nil {
			return err
		}

		requestPath := "/page/" + strings.ReplaceAll(relPath, "\\", "/")
		current[requestPath] = struct{}{}
		debug.Log(debug.DebugVerbose, "Registering page handler", "path", requestPath, "file", path)

		if err := r.destination.RegisterRequestHandler(
			requestPath,
			func(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) []byte {
				return r.servePage(path, data, requestID, linkID, remoteIdentity, requestedAt)
			},
			destination.AllowAll,
			nil,
		); err != nil {
			debug.Log(debug.DebugError, "Failed to register page handler", "path", requestPath, "error", err)
			return err
		}
		return nil
	})
	if walkErr != nil {
		debug.Log(debug.DebugError, "Failed to walk pages directory", "error", walkErr)
		return
	}

	debug.Log(debug.DebugInfo, "Pages directory synced", "handlers", len(current))

	for path := range r.registeredPagePaths {
		if _, keep := current[path]; !keep {
			if r.destination.DeregisterRequestHandler(path) {
				debug.Log(debug.DebugInfo, "Deregistered removed page handler", "path", path)
			}
		}
	}
	r.registeredPagePaths = current
}

func (r *Reticulum) syncFileHandlers() {
	r.staticFilesMu.Lock()
	defer r.staticFilesMu.Unlock()

	filesDir := r.filesPath
	if err := os.MkdirAll(filesDir, 0750); err != nil { // #nosec G301
		debug.Log(debug.DebugError, "Failed to create files directory", "error", err)
		return
	}

	current := make(map[string]struct{})
	walkErr := filepath.Walk(filesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(filesDir, path)
		if err != nil {
			return err
		}

		requestPath := "/file/" + strings.ReplaceAll(relPath, "\\", "/")
		current[requestPath] = struct{}{}
		debug.Log(debug.DebugVerbose, "Registering file handler", "path", requestPath, "file", path)

		if err := r.destination.RegisterRequestHandlerAny(
			requestPath,
			func(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) any {
				return r.serveFile(path, data, requestID, linkID, remoteIdentity, requestedAt)
			},
			destination.AllowAll,
			nil,
		); err != nil {
			debug.Log(debug.DebugError, "Failed to register file handler", "path", requestPath, "error", err)
			return err
		}
		return nil
	})
	if walkErr != nil {
		debug.Log(debug.DebugError, "Failed to walk files directory", "error", walkErr)
		return
	}

	debug.Log(debug.DebugInfo, "Files directory synced", "handlers", len(current))

	for path := range r.registeredFilePaths {
		if _, keep := current[path]; !keep {
			if r.destination.DeregisterRequestHandler(path) {
				debug.Log(debug.DebugInfo, "Deregistered removed file handler", "path", path)
			}
		}
	}
	r.registeredFilePaths = current
}

func (r *Reticulum) servePage(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) []byte {
	debug.Log(debug.DebugInfo, "Serving page", "path", path, "request_id", fmt.Sprintf("%x", requestID))

	var filePath string
	if strings.HasPrefix(path, "/page/") {
		filePath = filepath.Join(r.pagesPath, strings.TrimPrefix(path, "/page/"))
	} else {
		filePath = filepath.Join(r.pagesPath, path)
	}

	filePath = filepath.Clean(filePath)
	pagesDir := filepath.Clean(r.pagesPath)

	if !strings.HasPrefix(filePath, pagesDir) {
		debug.Log(debug.DebugError, "Path traversal attempt detected", "path", path)
		return []byte(">Request Not Allowed\n\nYou are not authorized to access this resource.")
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		debug.Log(debug.DebugError, "Failed to read page", "path", filePath, "error", err)
		return []byte(">Page Not Found\n\nThe requested page could not be found.")
	}

	debug.Log(debug.DebugVerbose, "Page served successfully", "path", path, "size", len(content))
	return content
}

func (r *Reticulum) serveFile(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) any {
	debug.Log(debug.DebugInfo, "Serving file", "path", path, "request_id", fmt.Sprintf("%x", requestID))

	var filePath string
	if strings.HasPrefix(path, "/file/") {
		filePath = filepath.Join(r.filesPath, strings.TrimPrefix(path, "/file/"))
	} else {
		filePath = filepath.Join(r.filesPath, path)
	}

	filePath = filepath.Clean(filePath)
	filesDir := filepath.Clean(r.filesPath)

	if !strings.HasPrefix(filePath, filesDir) {
		debug.Log(debug.DebugError, "Path traversal attempt detected", "path", path)
		return []byte(">Request Not Allowed\n\nYou are not authorized to access this resource.")
	}

	file, err := os.Open(filePath)
	if err != nil {
		debug.Log(debug.DebugError, "Failed to open file", "path", filePath, "error", err)
		return []byte(">File Not Found\n\nThe requested file could not be found.")
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		debug.Log(debug.DebugError, "Failed to read file", "path", filePath, "error", err)
		return []byte(">Error Reading File\n\nAn error occurred while reading the file.")
	}

	fileName := filepath.Base(filePath)
	debug.Log(debug.DebugVerbose, "File served successfully", "path", path, "size", len(content), "name", fileName)
	return []any{fileName, content}
}

func (r *Reticulum) handleLinkPacket(l *link.Link, data []byte, pkt *packet.Packet) {
	debug.Log(debug.DebugInfo, "Received packet on link", "link_id", fmt.Sprintf("%x", l.GetLinkID()), "data_len", len(data))

	if len(data) < 16 {
		debug.Log(debug.DebugError, "Request too short")
		return
	}

	requestID := data[:16]
	requestPath := string(data[16:])

	debug.Log(debug.DebugInfo, "Processing request", "path", requestPath, "request_id", fmt.Sprintf("%x", requestID))

	r.destination.HandleRequest(requestPath, nil, requestID, l.GetLinkID(), nil, time.Now().Unix())
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
	case flagWasSet("log-level") && *logLevel >= debug.DebugCritical:
		debug.SetDebugLevel(*logLevel)
	case flagWasSet("log-level") && *logLevel != -1 && *logLevel < debug.DebugCritical:
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

func printPageserverStartupSummary(r *Reticulum) {
	h := fmt.Sprintf("%x", r.destination.GetHash())
	pages := make([]string, 0, len(r.registeredPagePaths))
	for p := range r.registeredPagePaths {
		pages = append(pages, p)
	}
	sort.Strings(pages)
	files := make([]string, 0, len(r.registeredFilePaths))
	for p := range r.registeredFilePaths {
		files = append(files, p)
	}
	sort.Strings(files)

	w := os.Stderr
	fmt.Fprintf(w, "pageserver node destination hash: %s\n", h)
	fmt.Fprintf(w, "  node name: %s\n", r.config.AppName)
	fmt.Fprintf(w, "  pages (%d): ", len(pages))
	if len(pages) == 0 {
		fmt.Fprintln(w, "(none)")
	} else {
		fmt.Fprintln(w, strings.Join(pages, ", "))
	}
	fmt.Fprintf(w, "  files (%d): ", len(files))
	if len(files) == 0 {
		fmt.Fprintln(w, "(none)")
	} else {
		fmt.Fprintln(w, strings.Join(files, ", "))
	}
}
