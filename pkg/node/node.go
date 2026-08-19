// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/buffer"
	"quad4/reticulum-go/pkg/channel"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/discovery"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/sandbox"
	"quad4/reticulum-go/pkg/sharedinstance"
	"quad4/reticulum-go/pkg/transport"
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

	lastNetworkDown time.Time
	networkPaused   bool
	pauseMode       PauseMode
	watchedDests    map[string][]byte
	watchMu         sync.RWMutex
	linkMgr         *linkManager
	discovery       *discovery.InterfaceDiscovery
	announcer       *discovery.InterfaceAnnouncer
}

// StartInterfaceDiscovery enables rnstransport interface discovery listening
// and starts the InterfaceAnnouncer when discoverable interfaces are configured.
func (n *Node) StartInterfaceDiscovery() {
	if n == nil || n.transport == nil || n.config == nil {
		return
	}
	listen := n.config.DiscoverInterfaces || discovery.HasDiscoverableInterfaces(n.config)
	if !listen {
		return
	}
	if n.discovery == nil {
		isBH := func(h []byte) bool {
			tab := n.transport.BlackholeTable()
			return tab != nil && tab.Has(h)
		}
		n.discovery = discovery.NewInterfaceDiscoveryWithBlackhole(n.transport, discovery.DefaultStampValue, nil, isBH)
		n.discovery.Start()
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
	}
	inst, err := sharedinstance.Attach(n.config, n.transport, hooks)
	if err != nil {
		return fmt.Errorf("shared instance: %w", err)
	}
	n.sharedInstance = inst
	if err := n.transport.InitializeNetworkIdentity(); err != nil {
		return fmt.Errorf("network identity: %w", err)
	}
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
	for _, buf := range n.buffers {
		_ = buf.Close()
	}
	for _, ch := range n.channels {
		_ = ch.Close()
	}
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
			if err := n.transport.RegisterInterface(client.GetName(), client); err != nil {
				debug.Log(debug.DebugError, "Failed to register spawned local client", "error", err)
				return
			}
			n.handleInterface(client)
		},
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
