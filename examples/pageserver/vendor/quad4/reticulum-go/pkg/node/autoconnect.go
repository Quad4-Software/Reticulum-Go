// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"bytes"
	"fmt"
	"path/filepath"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/discovery"
	"quad4/reticulum-go/pkg/interfaces"
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
	if discovery.IsYggIPv6(info.Info.ReachableOn) {
		return
	}
	if n.autoconnectExists(info) {
		debug.Log(debug.DebugVerbose, "Discovered interface already exists, not auto-connecting",
			"type", ifaceType, "name", info.Info.Name)
		return
	}

	name := autoconnectInterfaceName(info)
	eh := discovery.EndpointHash(info)
	peerCfg := n.autoconnectPeerConfig()
	peerCfg.IFACNetname = info.Info.IFACNetname
	peerCfg.IFACNetkey = info.Info.IFACNetkey

	switch ifaceType {
	case "I2PInterface":
		if n.autoconnectI2P(info, name, eh, peerCfg) {
			return
		}
	case "TCPServerInterface":
		n.autoconnectTCPClient(info, name, eh, peerCfg)
	case "BackboneInterface":
		n.autoconnectBackboneClient(info, name, eh, peerCfg)
	}
}

func autoconnectInterfaceName(info *discovery.ReceivedAnnounceInfo) string {
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
	for _, e := range n.acEntries {
		if e.iface == nil {
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
	n.acMu.Unlock()
	for _, e := range detach {
		n.teardownAutoconnect(e)
	}
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
	_ = e.iface.Stop()
	n.transport.UnregisterInterface(name)
	n.unregisterInterfaceBuffers(name)

	n.reloadMu.Lock()
	filtered := n.interfaces[:0]
	for _, iface := range n.interfaces {
		if iface != e.iface {
			filtered = append(filtered, iface)
		}
	}
	n.interfaces = filtered
	n.reloadMu.Unlock()

	n.acMu.Lock()
	kept := n.acEntries[:0]
	for _, cur := range n.acEntries {
		if cur != e {
			kept = append(kept, cur)
		}
	}
	n.acEntries = kept
	n.acMu.Unlock()
}
