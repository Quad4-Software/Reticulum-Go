// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/discovery"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
)

const autoconnectMonitorInterval = 5 * time.Second
const autoconnectDetachAfter = 12 * time.Second

type autoconnectEntry struct {
	iface     interfaces.Interface
	hash      []byte
	downSince time.Time
}

func (n *Node) discoveryStorageDir() string {
	if n == nil || n.config == nil || n.config.UseInMemoryStorage() {
		return ""
	}
	if n.config.ConfigPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(n.config.ConfigPath), "storage")
}

func (n *Node) onInterfaceDiscovered(info *discovery.ReceivedAnnounceInfo) {
	if n == nil || info == nil || n.config == nil {
		return
	}
	_ = discovery.PersistDiscoveredInterface(n.discoveryStorageDir(), info)
	if n.config.AutoconnectDiscoveredInterfaces <= 0 {
		return
	}
	n.autoconnect(info)
}

func (n *Node) autoconnectCount() int {
	if n == nil {
		return 0
	}
	n.acMu.Lock()
	defer n.acMu.Unlock()
	return len(n.acEntries)
}

func (n *Node) autoconnectCandidateIfaces() []interfaces.Interface {
	n.reloadMu.Lock()
	candidates := append([]interfaces.Interface(nil), n.interfaces...)
	n.reloadMu.Unlock()
	for _, iface := range candidates {
		if lister, ok := iface.(interfaces.I2PSpawnedLister); ok {
			candidates = append(candidates, lister.ListSpawnedPeers()...)
		}
	}
	return candidates
}

func (n *Node) autoconnectExists(info *discovery.ReceivedAnnounceInfo) bool {
	eh := discovery.EndpointHash(info)
	n.acMu.Lock()
	for _, e := range n.acEntries {
		if len(e.hash) > 0 && string(e.hash) == string(eh) {
			n.acMu.Unlock()
			return true
		}
	}
	n.acMu.Unlock()

	host := info.Info.ReachableOn
	port := info.Info.Port
	hasPort := info.Info.HasPort
	for _, iface := range n.autoconnectCandidateIfaces() {
		if interfaces.MatchesDiscoveredEndpoint(iface, eh, host, port, hasPort) {
			return true
		}
	}
	return false
}

func (n *Node) autoconnectPeerConfig() *common.InterfaceConfig {
	cfg := &common.InterfaceConfig{
		Enabled: true,
		Bitrate: 5_000_000,
	}
	if n.config.AutoconnectInterfaceGravitySet {
		cfg.Gravity = n.config.AutoconnectInterfaceGravity
		cfg.GravitySet = true
	}
	if n.config.AutoconnectInterfaceMode != "" {
		cfg.Mode = n.config.AutoconnectInterfaceMode
	} else if n.config.EnableTransport {
		cfg.Mode = "gateway"
	}
	if n.config.AutoconnectAnnouncesToInternalSet {
		cfg.AnnouncesToInternal = n.config.AutoconnectAnnouncesToInternal
		cfg.AnnouncesToInternalSet = true
	}
	return cfg
}

// autoconnectQualified applies the RNS 1.5.6 criteria: only interfaces
// announced from transport-enabled nodes qualify, and announces without a
// recognized TRANSPORT_IMPL, or below the minimum version for that
// implementation, are not auto-connected unless
// autoconnect_unverified_implementations is enabled.
func (n *Node) autoconnectQualified(info *discovery.ReceivedAnnounceInfo) bool {
	if !info.Info.Transport {
		return false
	}
	if n.config.AutoconnectUnverifiedImplementations {
		return true
	}
	impl := info.Info.TransportImpl
	if impl == "" {
		return false
	}
	minVer, ok := discovery.AutoconnectMinVersions[impl]
	if !ok {
		return false
	}
	if info.Info.TransportVers == "" {
		return false
	}
	return discovery.VersionAtLeast(info.Info.TransportVers, minVer)
}

func (n *Node) autoconnect(info *discovery.ReceivedAnnounceInfo) {
	if n == nil || n.config == nil || info == nil {
		return
	}
	limit := n.config.AutoconnectDiscoveredInterfaces
	if limit <= 0 {
		return
	}
	if n.autoconnectCount() >= limit {
		return
	}
	ifaceType := info.Info.Type
	if _, ok := discovery.AutoconnectTypes[ifaceType]; !ok {
		return
	}
	if !n.autoconnectQualified(info) {
		impl := "unknown implementation"
		if info.Info.TransportImpl != "" || info.Info.TransportVers != "" {
			impl = strings.TrimSpace(info.Info.TransportImpl + " " + info.Info.TransportVers)
		}
		debug.Log(debug.DebugVerbose,
			"Not auto-connecting discovered interface, auto-connect criteria not satisfied",
			"type", ifaceType, "name", info.Info.Name, "impl", impl)
		return
	}
	if discovery.IsYggIPv6(info.Info.ReachableOn) {
		return
	}
	// TCP and backbone peers need a reachable address. Without it the spawn
	// would succeed against an empty target and retry forever. I2P peers use
	// the peers field instead.
	if ifaceType != "I2PInterface" && info.Info.ReachableOn == "" {
		debug.Log(debug.DebugVerbose,
			"Not auto-connecting discovered interface, no reachable address",
			"type", ifaceType, "name", info.Info.Name)
		return
	}

	// Serialize the exists-check plus spawn section so two announces for the
	// same endpoint cannot race into two interfaces (Python autoconnect_lock).
	n.acSpawnMu.Lock()
	defer n.acSpawnMu.Unlock()

	if n.autoconnectExists(info) {
		debug.Log(debug.DebugVerbose, "Discovered interface already exists, not auto-connecting",
			"type", ifaceType, "name", info.Info.Name)
		return
	}

	name := n.autoconnectInterfaceName(info)
	if name != autoconnectBaseName(info) {
		debug.Log(debug.DebugInfo, "Auto-connect name collision, using sequential name",
			"announced", info.Info.Name, "name", name)
	}
	eh := discovery.EndpointHash(info)
	peerCfg := n.autoconnectPeerConfig()
	// Drop the invalidly persisted literal "None" IFAC values published by
	// unguarded node-side configuration (RNS 1.5.5 sanitization).
	peerCfg.IFACNetname = discovery.SanitizeIFACValue(info.Info.IFACNetname)
	peerCfg.IFACNetkey = discovery.SanitizeIFACValue(info.Info.IFACNetkey)

	switch ifaceType {
	case "I2PInterface":
		if n.autoconnectI2P(info, name, eh, peerCfg) {
			return
		}
	case "TCPServerInterface":
		n.autoconnectTCPClient(info, name, eh, peerCfg)
	case "BackboneInterface":
		if discovery.BackboneSupported() {
			n.autoconnectBackboneClient(info, name, eh, peerCfg)
		} else {
			debug.Log(debug.DebugInfo,
				"BackboneInterface is not supported on this platform, auto-connecting using TCPClientInterface",
				"name", name)
			n.autoconnectTCPClient(info, name, eh, peerCfg)
		}
	}
}

// autoconnectBaseName derives the announced interface name without collision
// resolution.
func autoconnectBaseName(info *discovery.ReceivedAnnounceInfo) string {
	if info == nil {
		return "Discovered interface"
	}
	base := info.Info.Name
	if base == "" {
		base = "Discovered " + info.Info.Type
	}
	spec := info.Info.ReachableOn
	if info.Info.HasPort {
		spec = fmt.Sprintf("%s:%d", spec, info.Info.Port)
	}
	if spec == "" {
		return base
	}
	return fmt.Sprintf("%s (%s)", base, spec)
}

// autoconnectInterfaceName resolves name collisions against all registered
// interface names with sequential numbering, matching Python
// InterfaceDiscovery.autoconnect_interface_name (RNS 1.5.5).
func (n *Node) autoconnectInterfaceName(info *discovery.ReceivedAnnounceInfo) string {
	name := autoconnectBaseName(info)
	if n == nil || n.transport == nil {
		return name
	}
	if existing, err := n.transport.GetInterface(name); err != nil || existing == nil {
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s (%d)", name, i)
		if existing, err := n.transport.GetInterface(candidate); err != nil || existing == nil {
			return candidate
		}
	}
}

func (n *Node) autoconnectBackboneClient(info *discovery.ReceivedAnnounceInfo, name string, eh []byte, peerCfg *common.InterfaceConfig) {
	peerCfg.Type = "BackboneClientInterface"
	peerCfg.TargetHost = info.Info.ReachableOn
	peerCfg.TargetPort = int(info.Info.Port)

	created, err := interfaces.NewFromConfigWithContext(name, peerCfg, n.fromConfigContext())
	if err != nil {
		debug.Log(debug.DebugError, "Autoconnect create failed", "error", err)
		return
	}
	client, ok := created.(*interfaces.BackboneClientInterface)
	if !ok {
		debug.Log(debug.DebugError, "Autoconnect unexpected interface type", "got", fmt.Sprintf("%T", created))
		_ = created.Stop()
		return
	}
	client.AutoconnectHash = append([]byte(nil), eh...)
	client.AutoconnectSource = append([]byte(nil), info.RemoteIdentity...)

	if err := client.Start(); err != nil {
		debug.Log(debug.DebugError, "Autoconnect start failed", "error", err)
		return
	}
	if err := n.transport.RegisterInterface(client.GetName(), client); err != nil {
		debug.Log(debug.DebugError, "Autoconnect register failed", "error", err)
		_ = client.Stop()
		return
	}
	if !n.trackAutoconnect(client, eh, info.Info.Type, name, info.Info.ReachableOn, info.Info.Port) {
		n.transport.UnregisterInterface(client.GetName())
		_ = client.Stop()
		return
	}

	n.reloadMu.Lock()
	n.interfaces = append(n.interfaces, client)
	n.reloadMu.Unlock()
}

func (n *Node) autoconnectTCPClient(info *discovery.ReceivedAnnounceInfo, name string, eh []byte, peerCfg *common.InterfaceConfig) {
	peerCfg.Type = "TCPClientInterface"
	peerCfg.TargetHost = info.Info.ReachableOn
	peerCfg.TargetPort = int(info.Info.Port)

	created, err := interfaces.NewFromConfigWithContext(name, peerCfg, n.fromConfigContext())
	if err != nil {
		debug.Log(debug.DebugError, "Autoconnect create failed", "error", err)
		return
	}
	client, ok := created.(*interfaces.TCPClientInterface)
	if !ok {
		debug.Log(debug.DebugError, "Autoconnect unexpected interface type", "got", fmt.Sprintf("%T", created))
		_ = created.Stop()
		return
	}
	client.AutoconnectHash = append([]byte(nil), eh...)
	client.AutoconnectSource = append([]byte(nil), info.RemoteIdentity...)

	if err := client.Start(); err != nil {
		debug.Log(debug.DebugError, "Autoconnect start failed", "error", err)
		return
	}
	if err := n.transport.RegisterInterface(client.GetName(), client); err != nil {
		debug.Log(debug.DebugError, "Autoconnect register failed", "error", err)
		_ = client.Stop()
		return
	}
	if !n.trackAutoconnect(client, eh, info.Info.Type, name, info.Info.ReachableOn, info.Info.Port) {
		n.transport.UnregisterInterface(client.GetName())
		_ = client.Stop()
		return
	}

	n.reloadMu.Lock()
	n.interfaces = append(n.interfaces, client)
	n.reloadMu.Unlock()
}

func (n *Node) trackAutoconnect(iface interfaces.Interface, eh []byte, ifaceType, name, host string, port int64) bool {
	if n == nil || n.config == nil || iface == nil {
		return false
	}
	limit := n.config.AutoconnectDiscoveredInterfaces
	n.acMu.Lock()
	if len(n.acEntries) >= limit {
		n.acMu.Unlock()
		return false
	}
	for _, e := range n.acEntries {
		if len(eh) > 0 && len(e.hash) > 0 && bytes.Equal(e.hash, eh) {
			n.acMu.Unlock()
			return false
		}
	}
	n.acEntries = append(n.acEntries, &autoconnectEntry{iface: iface, hash: eh})
	n.acMu.Unlock()

	n.handleInterface(iface)
	n.wireConnectivityHooks(iface)
	n.ensureAutoconnectMonitor()
	debug.Log(debug.DebugInfo, "Auto-connecting discovered interface",
		"type", ifaceType, "name", name, "host", host, "port", port)
	return true
}

func (n *Node) drainAutoconnectEntries() {
	if n == nil {
		return
	}
	n.acMu.Lock()
	entries := append([]*autoconnectEntry(nil), n.acEntries...)
	n.acEntries = nil
	n.acMu.Unlock()
	for _, e := range entries {
		n.teardownAutoconnect(e)
	}
}

func (n *Node) reconnectPersistedAutoconnect() {
	if n == nil || n.config == nil || n.config.AutoconnectDiscoveredInterfaces <= 0 {
		return
	}
	list, err := discovery.LoadPersistedInterfaces(n.discoveryStorageDir())
	if err != nil {
		debug.Log(debug.DebugVerbose, "Load persisted discovery interfaces failed", "error", err)
		return
	}
	for _, info := range list {
		if !info.Info.Transport {
			continue
		}
		if n.autoconnectCount() >= n.config.AutoconnectDiscoveredInterfaces {
			break
		}
		n.autoconnect(info)
	}
}

func (n *Node) ensureAutoconnectMonitor() {
	n.acMu.Lock()
	defer n.acMu.Unlock()
	if n.acMonitorRunning {
		return
	}
	n.acMonitorRunning = true
	n.acMonitorStop = make(chan struct{})
	go n.autoconnectMonitorLoop(n.acMonitorStop)
}

func (n *Node) stopAutoconnectMonitor() {
	n.acMu.Lock()
	stop := n.acMonitorStop
	n.acMonitorRunning = false
	n.acMonitorStop = nil
	n.acMu.Unlock()
	if stop != nil {
		close(stop)
	}
}

func (n *Node) autoconnectMonitorLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(autoconnectMonitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			n.autoconnectMonitorTick()
		}
	}
}

func (n *Node) autoconnectMonitorTick() {
	n.acMu.Lock()
	now := time.Now()
	var detach []*autoconnectEntry
	var unmonitor []*autoconnectEntry
	for _, e := range n.acEntries {
		if e.iface == nil {
			continue
		}
		// RNS 1.5.5: a monitored auto-connected interface that was manually
		// detached leaves the transport registry; drop it from monitoring
		// instead of tearing it down again.
		if gone, gerr := n.transport.GetInterface(e.iface.GetName()); gerr != nil || gone == nil || gone != common.NetworkInterface(e.iface) {
			debug.Log(debug.DebugVerbose, "A monitored auto-connected interface was manually detached, removing from monitoring",
				"name", e.iface.GetName())
			unmonitor = append(unmonitor, e)
			continue
		}
		if e.iface.IsOnline() {
			e.downSince = time.Time{}
			continue
		}
		if e.downSince.IsZero() {
			e.downSince = now
			continue
		}
		if now.Sub(e.downSince) >= autoconnectDetachAfter {
			detach = append(detach, e)
		}
	}
	for _, e := range unmonitor {
		n.untrackAutoconnectLocked(e)
	}
	n.acMu.Unlock()
	for _, e := range unmonitor {
		n.reloadMu.Lock()
		kept := n.interfaces[:0]
		for _, cur := range n.interfaces {
			if cur != e.iface {
				kept = append(kept, cur)
			}
		}
		n.interfaces = kept
		n.reloadMu.Unlock()
	}
	for _, e := range detach {
		n.teardownAutoconnect(e)
	}
}

// untrackAutoconnectLocked removes e from acEntries. acMu must be held.
func (n *Node) untrackAutoconnectLocked(e *autoconnectEntry) {
	kept := n.acEntries[:0]
	for _, cur := range n.acEntries {
		if cur != e {
			kept = append(kept, cur)
		}
	}
	n.acEntries = kept
}

func (n *Node) teardownAutoconnect(e *autoconnectEntry) {
	if e == nil || e.iface == nil {
		return
	}
	name := e.iface.GetName()
	debug.Log(debug.DebugVerbose, "Tearing down auto-connected interface", "name", name)
	if hook, ok := e.iface.(interface{ DetachAutoconnectFromParent() }); ok {
		hook.DetachAutoconnectFromParent()
	}
	// Route through the shared drop path used by interface management
	// (Python teardown_interface -> _detach_interface, RNS 1.5.5).
	n.dropInterface(e.iface)
}
