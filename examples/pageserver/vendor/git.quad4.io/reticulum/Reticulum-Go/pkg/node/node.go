// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/backbone"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/buffer"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/channel"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/discovery"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/hostcap"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/sandbox"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/sharedinstance"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

// PauseMode controls how OnNetworkLost affects interfaces.
type PauseMode int

const (
	PauseModeDisable PauseMode = iota
	PauseModeStop
)

// LinkReconnectOptions configures optional automatic link re-establishment.
type LinkReconnectOptions struct {
	MaxAttempts int
	Backoff     time.Duration
}

// Node orchestrates transport, interfaces, and network lifecycle for embedders.
type Node struct {
	config         *common.ReticulumConfig
	transport      *transport.Transport
	sharedInstance *sharedinstance.Instance
	interfaces     []interfaces.Interface
	channels       map[string]*channel.Channel
	buffers        map[string]*buffer.Buffer
	reloadMu       sync.Mutex
	wiringMu       sync.Mutex

	lastNetworkDown time.Time
	networkPaused   bool
	pauseMode       PauseMode
	watchedDests    map[string][]byte
	watchMu         sync.RWMutex
	linkMgr         *linkManager
	discovery       *discovery.InterfaceDiscovery
	announcer       *discovery.InterfaceAnnouncer

	bhUpdaterMu      sync.Mutex
	bhUpdaterRunning bool
	bhUpdaterLast    map[[16]byte]time.Time
	bhUpdaterStop    chan struct{}

	acMu             sync.Mutex
	acSpawnMu        sync.Mutex
	acEntries        []*autoconnectEntry
	acMonitorRunning bool
	acMonitorStop    chan struct{}

	netmonMu   sync.Mutex
	netmonStop chan struct{}

	awareDriver interfaces.AwareDriver
}

// SetAwareDriver installs the host-supplied WiFi Aware session driver before
// Start. Android builds inject the gomobile bridge here. When unset, an
// AwareInterface in config fails at Start.
func (n *Node) SetAwareDriver(d interfaces.AwareDriver) {
	n.awareDriver = d
}

// StartInterfaceDiscovery enables rnstransport interface discovery listening
// and starts the InterfaceAnnouncer when discoverable interfaces are configured.
func (n *Node) StartInterfaceDiscovery() {
	if n == nil || n.transport == nil || n.config == nil {
		return
	}
	listen := n.config.DiscoverInterfaces || discovery.HasDiscoverableInterfaces(n.config) ||
		n.config.AutoconnectDiscoveredInterfaces > 0
	if !listen {
		return
	}
	if n.discovery == nil {
		n.transport.WantsDiscoveryDestination()
		isBH := func(h []byte) bool {
			tab := n.transport.BlackholeTable()
			return tab != nil && tab.Has(h)
		}
		var onDisc func(*discovery.ReceivedAnnounceInfo)
		if n.config.DiscoverInterfaces || n.config.AutoconnectDiscoveredInterfaces > 0 {
			onDisc = n.onInterfaceDiscovered
		}
		n.discovery = discovery.NewInterfaceDiscoveryWithBlackhole(n.transport, discovery.DefaultStampValue, onDisc, isBH)
		n.discovery.Start()
		n.reconnectPersistedAutoconnect()
	}
	if n.announcer != nil || !discovery.HasDiscoverableInterfaces(n.config) {
		return
	}
	id := n.transport.NetworkIdentity()
	if id == nil {
		id = n.transport.TransportIdentity()
	}
	if id == nil {
		debug.Log(debug.DebugError, "Interface discovery announcer skipped: no identity")
		return
	}
	ann, err := discovery.NewInterfaceAnnouncer(n.transport, n.config, id)
	if err != nil {
		debug.Log(debug.DebugError, "Interface discovery announcer failed", "error", err)
		return
	}
	n.announcer = ann
	n.announcer.Start()
}

// New constructs a Node from configuration without starting it.
func New(cfg *common.ReticulumConfig) (*Node, error) {
	if cfg == nil {
		cfg = common.DefaultConfig()
	}
	sandbox.SetExecRlimits(cfg.SandboxExecRlimits)
	if _, err := backbone.Init(backbone.ParseBackend(cfg.BackboneIO)); err != nil {
		return nil, fmt.Errorf("backbone I/O hub: %w", err)
	}
	// Upstream 606a1691: a discoverable interface on a non-transport
	// instance without static_transport_identity announces under the
	// ephemeral transport identity, so every restart would show it as a
	// new node. Discovery is disabled for that interface instead of
	// silently pinning the identity. Upstream applies this during
	// config parsing, before the enabled and type checks.
	if !cfg.EnableTransport && !cfg.StaticTransportIdentity {
		for name, iface := range cfg.Interfaces {
			if iface == nil || !iface.Discoverable {
				continue
			}
			debug.Log(debug.DebugWarning, "The interface \""+name+"\" was configured discoverable, but this instance has transport disabled")
			debug.Log(debug.DebugWarning, "and is using an ephemeral transport identity. To make interfaces discoverable for non-transport")
			debug.Log(debug.DebugWarning, "instances, explicitly enable using static transport identity in the config file,")
			debug.Log(debug.DebugWarning, "by adding \"static_transport_identity = yes\" to the [reticulum] section.")
			iface.Discoverable = false
		}
	}
	t := transport.NewTransport(cfg)
	n := &Node{
		config:       cfg,
		transport:    t,
		interfaces:   make([]interfaces.Interface, 0),
		channels:     make(map[string]*channel.Channel),
		buffers:      make(map[string]*buffer.Buffer),
		pauseMode:    PauseModeDisable,
		watchedDests: make(map[string][]byte),
	}
	ctx := n.fromConfigContext()
	for name, ifaceConfig := range cfg.Interfaces {
		if !ifaceConfig.Enabled {
			continue
		}
		iface, err := interfaces.NewFromConfigWithContext(name, ifaceConfig, ctx)
		if err != nil {
			if cfg.PanicOnInterfaceErr {
				return nil, fmt.Errorf("failed to create interface %s: %w", name, err)
			}
			debug.Log(debug.DebugError, "Error creating interface", "name", name, "error", err)
			continue
		}
		n.interfaces = append(n.interfaces, iface)
	}
	return n, nil
}

// Transport returns the underlying transport.
func (n *Node) Transport() *transport.Transport {
	return n.transport
}

// Config returns the active configuration.
func (n *Node) Config() *common.ReticulumConfig {
	return n.config
}

// Interfaces returns the configured interface list.
func (n *Node) Interfaces() []interfaces.Interface {
	return n.interfaces
}

// Start starts transport and network interfaces.
func (n *Node) Start() error {
	transportNode := n.config != nil && n.config.EnableTransport
	hostcap.Start(context.Background(), transportNode)
	if err := n.transport.Start(); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}
	if err := n.transport.InitializePathRequestHandler(); err != nil {
		return fmt.Errorf("path request handler: %w", err)
	}
	if err := n.transport.InitializeProbeDestination(); err != nil {
		return fmt.Errorf("probe destination: %w", err)
	}
	hooks := sharedinstance.Hooks{
		RegisterInterface: n.transport.RegisterInterface,
		HandleInterface:   n.handleInterface,
		ManageInterface:   n.manageInterfaceRPC,
	}
	inst, err := sharedinstance.Attach(n.config, n.transport, hooks)
	if err != nil {
		return fmt.Errorf("shared instance: %w", err)
	}
	n.sharedInstance = inst
	if err := n.transport.InitializeNetworkIdentity(); err != nil {
		return fmt.Errorf("network identity: %w", err)
	}
	n.loadConfigDirTables()
	if !inst.OwnsNetworkInterfaces() {
		debug.Log(debug.DebugInfo, "Using existing local shared Reticulum instance, skipping configured network interfaces")
		return nil
	}
	if err := n.startInterfaces(); err != nil {
		return err
	}
	if err := n.transport.InitializeRemoteManagement(); err != nil {
		return fmt.Errorf("remote management: %w", err)
	}
	if err := n.transport.InitializeBlackholePublish(); err != nil {
		return fmt.Errorf("blackhole publish: %w", err)
	}
	n.StartBlackholeUpdater()
	return nil
}

func (n *Node) startInterfaces() error {
	type result struct {
		iface interfaces.Interface
		err   error
	}
	results := make(chan result, len(n.interfaces))
	for _, iface := range n.interfaces {
		go func(iface interfaces.Interface) {
			results <- result{iface: iface, err: iface.Start()}
		}(iface)
	}
	started := make([]interfaces.Interface, 0, len(n.interfaces))
	for range len(n.interfaces) {
		res := <-results
		if res.err != nil {
			if n.config.PanicOnInterfaceErr {
				return fmt.Errorf("failed to start interface %s: %w", res.iface.GetName(), res.err)
			}
			debug.Log(debug.DebugError, "Error starting interface", "name", res.iface.GetName(), "error", res.err)
			continue
		}
		started = append(started, res.iface)
	}
	for _, iface := range started {
		ni, ok := iface.(common.NetworkInterface)
		if !ok {
			continue
		}
		if err := n.transport.RegisterInterface(iface.GetName(), ni); err != nil {
			debug.Log(debug.DebugError, "Failed to register interface", "name", iface.GetName(), "error", err)
			continue
		}
		n.handleInterface(ni)
		n.wireConnectivityHooks(iface)
		if lc, ok := iface.(*interfaces.LocalClientInterface); ok && lc.IsSharedInstanceClient() {
			n.transport.SetConnectedToSharedInstance(true)
			if n.config != nil {
				n.config.ConnectedToSharedInstance = true
			}
		}
	}
	n.interfaces = started
	if n.config != nil && n.config.WatchInterfaces {
		n.startInterfaceMonitor()
	}
	n.StartInterfaceDiscovery()
	return nil
}

// Stop shuts down interfaces and transport.
func (n *Node) Stop() error {
	hostcap.Stop()
	n.netmonMu.Lock()
	if n.netmonStop != nil {
		close(n.netmonStop)
		n.netmonStop = nil
	}
	n.netmonMu.Unlock()
	n.stopBlackholeUpdater()
	n.stopAutoconnectMonitor()
	n.drainAutoconnectEntries()
	n.reloadMu.Lock()
	defer n.reloadMu.Unlock()
	if n.announcer != nil {
		n.announcer.Stop()
		n.announcer = nil
	}
	if n.discovery != nil {
		n.discovery.Stop()
		n.discovery = nil
	}
	if n.sharedInstance != nil {
		n.sharedInstance.Close()
		n.sharedInstance = nil
	}
	n.wiringMu.Lock()
	for _, buf := range n.buffers {
		_ = buf.Close()
	}
	for _, ch := range n.channels {
		_ = ch.Close()
	}
	n.wiringMu.Unlock()
	for _, iface := range n.interfaces {
		_ = iface.Stop()
	}
	if n.transport != nil {
		if err := n.transport.Close(); err != nil {
			return err
		}
	}
	backbone.Shutdown()
	return nil
}

// WatchDestination registers a destination hash for path refresh on wake.
func (n *Node) WatchDestination(hash []byte) error {
	if len(hash) != 16 {
		return errors.New("destination hash must be 16 bytes")
	}
	key := hex.EncodeToString(hash)
	n.watchMu.Lock()
	n.watchedDests[key] = append([]byte(nil), hash...)
	n.watchMu.Unlock()
	return nil
}

// EnableLinkAutoReconnect enables automatic link re-establishment.
func (n *Node) EnableLinkAutoReconnect(opts LinkReconnectOptions) {
	if n.linkMgr == nil {
		n.linkMgr = newLinkManager(n.transport, opts)
	}
}

func defaultGravityFromConfig(cfg *common.ReticulumConfig) int {
	if cfg != nil && cfg.DefaultGravitySet {
		return cfg.DefaultGravity
	}
	return 0
}

// nodeConfigDir returns the directory holding this node's config files.
func (n *Node) nodeConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	configDir := filepath.Join(homeDir, ".reticulum-go")
	if n.config != nil && n.config.ConfigPath != "" {
		configDir = filepath.Dir(n.config.ConfigPath)
	}
	return configDir
}

// parseBlockedIPs extracts valid IP addresses from a blocked_ips file body:
// one entry per line, blank lines and # comments skipped.
func parseBlockedIPs(raw []byte) []string {
	var out []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		ip := strings.TrimSpace(line)
		if ip == "" || strings.HasPrefix(ip, "#") {
			continue
		}
		if net.ParseIP(ip) != nil {
			out = append(out, ip)
		}
	}
	return out
}

// loadConfigDirTables reads the config-dir tables RNS 1.6.1 loads at
// startup: blocked_ips (permanent backbone blocks), known_destination_types
// and flagged_destination_types (name-hash display names).
func (n *Node) loadConfigDirTables() {
	configDir := n.nodeConfigDir()
	if configDir == "" {
		return
	}
	if raw, err := os.ReadFile(filepath.Join(configDir, "blocked_ips")); err == nil { // #nosec G304 -- path under the node's own config dir
		ips := parseBlockedIPs(raw)
		for _, ip := range ips {
			interfaces.BlockPermanently(ip)
		}
		debug.Log(debug.DebugVerbose, "Loaded blocked IPs", "count", len(ips))
	}
	for _, table := range []struct {
		file     string
		register func([]byte, string)
	}{
		{"known_destination_types", n.transport.RegisterKnownDestinationType},
		{"flagged_destination_types", n.transport.RegisterFlaggedDestinationType},
	} {
		raw, err := os.ReadFile(filepath.Join(configDir, table.file)) // #nosec G304 -- path under the node's own config dir
		if err != nil {
			continue
		}
		for line := range strings.SplitSeq(string(raw), "\n") {
			name := strings.TrimSpace(line)
			if name == "" || strings.HasPrefix(name, "#") {
				continue
			}
			h := sha256.Sum256([]byte(name))
			table.register(h[:10], name)
		}
	}
}

func (n *Node) fromConfigContext() *interfaces.FromConfigContext {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	configDir := filepath.Join(homeDir, ".reticulum-go")
	if n.config != nil && n.config.ConfigPath != "" {
		configDir = filepath.Dir(n.config.ConfigPath)
	}
	storage := filepath.Join(configDir, "storage")
	return &interfaces.FromConfigContext{
		I2PStoragePath:        storage,
		ConfigDir:             configDir,
		TransportID:           n.transport.TransportIdentityHash(),
		WatchInterfaces:       n.config != nil && n.config.WatchInterfaces,
		DiscoverInterfaces:    n.config != nil && n.config.DiscoverInterfaces,
		PanicOnInterfaceError: n.config != nil && n.config.PanicOnInterfaceErr,
		DefaultGravity:        defaultGravityFromConfig(n.config),
		BackboneHub:           backbone.Get(),
		SpawnBackbone: func(client *interfaces.BackboneClientInterface) {
			if err := n.transport.RegisterInterface(client.GetName(), client); err != nil {
				debug.Log(debug.DebugError, "Failed to register spawned backbone client", "error", err)
				return
			}
			n.handleInterface(client)
		},
		SpawnLocal: func(client *interfaces.LocalClientInterface) {
			name := client.GetName()
			if err := n.transport.RegisterInterface(name, client); err != nil {
				debug.Log(debug.DebugError, "Failed to register spawned local client", "error", err)
				_ = client.Stop()
				return
			}
			n.handleInterface(client)
			client.SetDisconnectHooks(func() {
				n.transport.UnregisterInterface(name)
				n.unregisterInterfaceBuffers(name)
			}, nil)
		},
		SpawnTCP: func(client *interfaces.TCPServerClientInterface) {
			name := client.GetName()
			if err := n.transport.RegisterInterface(name, client); err != nil {
				debug.Log(debug.DebugError, "Failed to register spawned TCP client", "error", err)
				_ = client.Stop()
				return
			}
			n.handleInterface(client)
			client.SetDisconnectHooks(func() {
				n.transport.UnregisterInterface(name)
				n.unregisterInterfaceBuffers(name)
			})
		},
		SpawnAware: func(peer *interfaces.AwarePeerInterface) {
			name := peer.GetName()
			if err := n.transport.RegisterInterface(name, peer); err != nil {
				debug.Log(debug.DebugError, "Failed to register spawned aware peer", "error", err)
				_ = peer.Stop()
				return
			}
			n.handleInterface(peer)
		},
		AwareDriver: n.awareDriver,
		RegisterPeer: func(name string, peer common.NetworkInterface) error {
			return n.transport.RegisterInterface(name, peer)
		},
		UnregisterPeer: func(name string) {
			n.transport.UnregisterInterface(name)
			n.unregisterInterfaceBuffers(name)
		},
		SetupPeer: n.handleInterface,
		SynthesizeTunnel: func(peer interfaces.TunnelPeer) {
			_ = n.transport.SynthesizeTunnel(peer)
		},
		VoidTunnel: func(peer interfaces.TunnelPeer) {
			n.transport.VoidTunnel(peer)
		},
	}
}

func (n *Node) wireConnectivityHooks(iface interfaces.Interface) {
	notifier, ok := iface.(interfaces.ConnectivityNotifier)
	if !ok {
		return
	}
	notifier.SetConnectivityHooks(
		func() { debug.Log(debug.DebugVerbose, "Interface connectivity down", "name", iface.GetName()) },
		func() { debug.Log(debug.DebugVerbose, "Interface connectivity up", "name", iface.GetName()) },
	)
}

func (n *Node) ownsInterfaces() bool {
	return n.sharedInstance == nil || n.sharedInstance.OwnsNetworkInterfaces()
}

// RegisterLink registers a link for automatic re-establishment when enabled.
func (n *Node) RegisterLink(l *link.Link) {
	if n.linkMgr != nil {
		n.linkMgr.Register(l)
	}
}
