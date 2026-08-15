// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package pageserver

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"quad4/msgpack/v5/pkg/msgpack"

	"quad4/reticulum-go/pkg/buffer"
	"quad4/reticulum-go/pkg/channel"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/sandbox"
	"quad4/reticulum-go/pkg/transport"

	"quad4/reticulum-go/pkg/pageserver/dynamicpage"
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

	pageStatsMu       sync.RWMutex
	pageStats         map[string]int64
	pageStatsDisabled bool

	announceEveryMinutes int
}

type announceRecord struct {
	timestamp int64
	appData   []byte
}

// NewReticulum loads or creates identity, opens the destination, wires link and static handlers, and builds interfaces from cfg.
func NewReticulum(cfg *common.ReticulumConfig, opts Options) (*Reticulum, error) {
	if cfg == nil {
		cfg = common.DefaultConfig()
	}
	sandbox.SetExecRlimits(cfg.SandboxExecRlimits)

	if opts.NodeDisplayName == "" {
		opts.NodeDisplayName = AppName
	}
	cfg.AppName = opts.NodeDisplayName
	if cfg.AppAspect == "" {
		cfg.AppAspect = AppAspect
	}

	if err := InitializeDirectories(); err != nil {
		return nil, fmt.Errorf("failed to initialize directories: %v", err)
	}
	debug.Log(debug.DebugInfo, "Directories initialized")

	t := transport.NewTransport(cfg)
	debug.Log(debug.DebugInfo, "Transport initialized")

	identityPath := ResolveIdentityPath(opts.IdentityFileOverride)

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

	var pageStats map[string]int64
	if !opts.DisablePageStats {
		pageStats = make(map[string]int64)
	}

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

		pagesPath:            opts.PagesDir,
		filesPath:            opts.FilesDir,
		establishedLinks:     make(map[string]*link.Link),
		pagesRefreshInterval: opts.PageRefreshInterval,
		filesRefreshInterval: opts.FileRefreshInterval,
		refreshStop:          make(chan struct{}),

		pageStats:         pageStats,
		pageStatsDisabled: opts.DisablePageStats,

		announceEveryMinutes: opts.AnnounceIntervalMinutes,
	}

	dest.AcceptsLinks(true)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	ratchetPath := destPrivateRatchetPath(filepath.Join(homeDir, ".reticulum-go"), dest, ident)
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

func (r *Reticulum) Transport() *transport.Transport {
	return r.transport
}

func (r *Reticulum) MonitorInterfaces() {
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

func InitializeDirectories() error {
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
		nodeName = AppName
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

	interval := time.Duration(r.announceEveryMinutes) * time.Minute
	if r.announceEveryMinutes <= 0 {
		debug.Log(debug.DebugInfo, "Periodic announces disabled (announce-interval 0). Only the initial announce was sent")
	} else {
		if interval < MinAnnounceInterval {
			debug.Log(debug.DebugCritical,
				"Configured announce-interval is below the minimum. Clamping to avoid flooding peers",
				"requested_minutes", r.announceEveryMinutes,
				"min", FormatDuration(MinAnnounceInterval),
			)
			interval = MinAnnounceInterval
		}
		debug.Log(debug.DebugInfo, "Starting periodic announce loop",
			"interval_minutes", r.announceEveryMinutes,
			"interval", FormatDuration(interval),
		)
		go func(period time.Duration) {
			ticker := time.NewTicker(period)
			defer ticker.Stop()
			for range ticker.C {
				debug.Log(debug.DebugInfo, "Sending periodic announce",
					"every", FormatDuration(period),
				)
				if err := r.destination.Announce(false, nil, nil); err != nil {
					debug.Log(debug.DebugCritical, "Could not send announce", "error", err)
				}
			}
		}(interval)
	}

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

func (h *AnnounceHandler) ReceivedAnnounce(destHash []byte, id any, appData []byte, hops uint8) error {
	debug.Log(debug.DebugInfo, "Received announce", "hash", fmt.Sprintf("%x", destHash))
	debug.Log(debug.DebugPackets, "Raw announce data", "data", fmt.Sprintf("%x", appData))
	debug.Log(debug.DebugInfo, "MAIN HANDLER: Received announce", "hash", fmt.Sprintf("%x", destHash), "appData_len", len(appData))

	var isNode bool
	var nodeEnabled bool
	var nodeTimestamp int64
	var nodeMaxSize int16

	if len(appData) > 0 {
		var decoded any
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
				nodeName = ClampDisplayName(nodeName, "")
				if nodeName != "" {
					debug.Log(debug.DebugInfo, "Parsed node name", "name", nodeName)
					debug.Log(debug.DebugInfo, "Announced node", "name", nodeName)
				}
			} else if !isNode {
				debug.Log(debug.DebugInfo, "Parsed structured appData", "value", fmt.Sprintf("%v", decoded))
			}
		} else {
			if utf8.Valid(appData) {
				nodeName := ClampDisplayName(string(appData), "")
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

	if announcedID, ok := id.(*identity.Identity); ok {
		debug.Log(debug.DebugAll, "Identity details")
		debug.Log(debug.DebugAll, "Identity hash", "hash", announcedID.GetHexHash())
		debug.Log(debug.DebugAll, "Identity public key", "key", fmt.Sprintf("%x", announcedID.GetPublicKey()))

		if ratchetPub := identity.GetRatchet(destHash); len(ratchetPub) > 0 {
			debug.Log(debug.DebugAll, "Announced ratchet ID", "id", fmt.Sprintf("%x", announcedID.GetRatchetID(ratchetPub)))
		}

		recordType := "peer"
		if isNode {
			recordType = "node"
			debug.Log(debug.DebugInfo, "Storing node in announce history", "enabled", nodeEnabled, "timestamp", nodeTimestamp, "maxsize", fmt.Sprintf("%dKB", nodeMaxSize))
		}

		h.reticulum.announceHistoryMu.Lock()
		h.reticulum.announceHistory[announcedID.GetHexHash()] = announceRecord{
			timestamp: time.Now().Unix(),
			appData:   appData,
		}
		h.reticulum.announceHistoryMu.Unlock()

		debug.Log(debug.DebugVerbose, "Stored announce in history", "type", recordType, "identity", announcedID.GetHexHash())
	}

	return nil
}

func (h *AnnounceHandler) ReceivePathResponses() bool {
	return true
}

func extractNodeName(decoded any) (string, bool) {
	if decoded == nil {
		return "", false
	}

	if str, ok := msgpackString(decoded); ok {
		str = strings.TrimSpace(str)
		return str, str != ""
	}

	arr, ok := decoded.([]any)
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

func parseNodeStatusAppData(decoded any) (enabled bool, timestamp int64, maxSizeKB int16, ok bool) {
	arr, ok := decoded.([]any)
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
	if maxSizeVal < math.MinInt16 || maxSizeVal > math.MaxInt16 {
		return false, 0, 0, false
	}

	return enabledVal, timestampVal, int16(maxSizeVal), true
}

func msgpackString(v any) (string, bool) {
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

func msgpackInt64(v any) (int64, bool) {
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

func (r *Reticulum) onLinkEstablished(linkInterface any) {
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

	l.SetLinkClosedCallback(func(closed *link.Link) {
		r.linksMutex.Lock()
		delete(r.establishedLinks, fmt.Sprintf("%x", closed.GetLinkID()))
		r.linksMutex.Unlock()
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
	r.syncBuiltInPageStatsHandler()
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

	var joined string
	if after, ok := strings.CutPrefix(path, "/page/"); ok {
		joined = filepath.Join(r.pagesPath, after)
	} else {
		joined = filepath.Join(r.pagesPath, path)
	}

	filePath, ok := resolveJailedPath(r.pagesPath, joined)
	if !ok {
		debug.Log(debug.DebugError, "Path traversal attempt detected", "path", path)
		return []byte(">Request Not Allowed\n\nYou are not authorized to access this resource.")
	}

	content, err := dynamicpage.ReadOrExecute(filePath, data, linkID, remoteIdentity)
	if err != nil {
		debug.Log(debug.DebugError, "Failed to read page", "path", filePath, "error", err)
		return []byte(">Page Not Found\n\nThe requested page could not be found.")
	}

	r.recordPageView(path)

	debug.Log(debug.DebugVerbose, "Page served successfully", "path", path, "size", len(content))
	return content
}

func (r *Reticulum) serveFile(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) any {
	debug.Log(debug.DebugInfo, "Serving file", "path", path, "request_id", fmt.Sprintf("%x", requestID))

	var joined string
	if after, ok := strings.CutPrefix(path, "/file/"); ok {
		joined = filepath.Join(r.filesPath, after)
	} else {
		joined = filepath.Join(r.filesPath, path)
	}

	filePath, ok := resolveJailedPath(r.filesPath, joined)
	if !ok {
		debug.Log(debug.DebugError, "Path traversal attempt detected", "path", path)
		return []byte(">Request Not Allowed\n\nYou are not authorized to access this resource.")
	}

	file, err := os.Open(filePath) // #nosec G304 -- filePath resolved and jail-validated by resolveJailedPath
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

func destPrivateRatchetPath(configRoot string, dest *destination.Destination, ident *identity.Identity) string {
	dir := filepath.Join(configRoot, "storage", "ratchets")
	path := filepath.Join(dir, hex.EncodeToString(dest.GetHash()))
	if ident == nil {
		return path
	}
	old := filepath.Join(dir, ident.GetHexHash())
	if path == old {
		return path
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if _, err := os.Stat(old); err != nil {
		return path
	}
	if err := os.Rename(old, path); err != nil {
		debug.Log(debug.DebugError, "Failed to rename ratchet file to destination hash", "error", err, "from", old, "to", path)
		return old
	}
	return path
}
