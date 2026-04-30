// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package interfaces

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
)

type DequeEntry struct {
	hash      [32]byte
	timestamp time.Time
}

type AutoInterface struct {
	BaseInterface
	groupID            []byte
	groupHash          []byte
	discoveryPort      int
	dataPort           int
	discoveryScope     string
	multicastAddrType  string
	mcastDiscoveryAddr string
	peers              map[string]*Peer
	linkLocalAddrs     []string
	adoptedInterfaces  map[string]*AdoptedInterface
	interfaceServers   map[string]*net.UDPConn
	discoveryServers   map[string]*net.UDPConn
	multicastEchoes    map[string]time.Time
	timedOutInterfaces map[string]time.Time
	allowedInterfaces  []string
	ignoredInterfaces  []string
	outboundConn       *net.UDPConn
	announceInterval   time.Duration
	peerJobInterval    time.Duration
	peeringTimeout     time.Duration
	mcastEchoTimeout   time.Duration
	mifDeque           []DequeEntry
	done               chan struct{}
	stopOnce           sync.Once
}

type AdoptedInterface struct {
	name          string
	linkLocalAddr string
	index         int
}

type Peer struct {
	ifaceName string
	lastHeard time.Time
	addr      *net.UDPAddr
}

func descopeLinkLocal(addr string) string {
	// Drop scope specifier expressed as %ifname (macOS)
	if i := strings.Index(addr, "%"); i != -1 {
		addr = addr[:i]
	}

	// Drop embedded scope specifier (NetBSD, OpenBSD)
	if strings.HasPrefix(addr, "fe80:") {
		parts := strings.Split(addr, ":")
		// Check for fe80:[scope]::...
		if len(parts) >= 3 && parts[2] == "" && parts[1] != "" {
			return "fe80::" + strings.Join(parts[3:], ":")
		}
	}
	return addr
}

func NewAutoInterface(name string, config *common.InterfaceConfig) (*AutoInterface, error) {
	groupID := DefaultGroupID
	if config.GroupID != "" {
		groupID = config.GroupID
	}

	discoveryScope := ScopeLink
	if config.DiscoveryScope != "" {
		discoveryScope = normalizeScope(config.DiscoveryScope)
	}

	multicastAddrType := McastAddrTypeTemporary
	if config.MulticastAddrType != "" {
		multicastAddrType = normalizeMulticastType(config.MulticastAddrType)
	}

	discoveryPort := DefaultDiscoveryPort
	if config.DiscoveryPort != 0 {
		discoveryPort = config.DiscoveryPort
	}

	dataPort := DefaultDataPort
	if config.DataPort != 0 {
		dataPort = config.DataPort
	}

	groupHash := sha256.Sum256([]byte(groupID))

	var gt strings.Builder
	gt.WriteString("0")
	for i := 1; i <= 6; i++ {
		gt.WriteString(fmt.Sprintf(":%02x%02x", groupHash[i*2], groupHash[i*2+1]))
	}
	mcastAddr := fmt.Sprintf("ff%s%s:%s", multicastAddrType, discoveryScope, gt.String())

	ai := &AutoInterface{
		BaseInterface: BaseInterface{
			Name:     name,
			Mode:     common.IFModeFull,
			Type:     common.IFTypeAuto,
			Online:   false,
			Enabled:  config.Enabled,
			Detached: false,
			In:       true,
			Out:      false,
			MTU:      HWMTU,
			Bitrate:  BitrateGuess,
		},
		groupID:            []byte(groupID),
		groupHash:          groupHash[:],
		discoveryPort:      discoveryPort,
		dataPort:           dataPort,
		discoveryScope:     discoveryScope,
		multicastAddrType:  multicastAddrType,
		mcastDiscoveryAddr: mcastAddr,
		peers:              make(map[string]*Peer),
		linkLocalAddrs:     make([]string, 0),
		adoptedInterfaces:  make(map[string]*AdoptedInterface),
		interfaceServers:   make(map[string]*net.UDPConn),
		discoveryServers:   make(map[string]*net.UDPConn),
		multicastEchoes:    make(map[string]time.Time),
		timedOutInterfaces: make(map[string]time.Time),
		allowedInterfaces:  make([]string, 0),
		ignoredInterfaces:  make([]string, 0),
		announceInterval:   AnnounceInterval,
		peerJobInterval:    PeerJobInterval,
		peeringTimeout:     PeeringTimeout,
		mcastEchoTimeout:   McastEchoTimeout,
		mifDeque:           make([]DequeEntry, 0, MultiIFDequeLen),
		done:               make(chan struct{}),
	}

	debug.Log(debug.DebugInfo, "AutoInterface configured", "name", name, "group", groupID, "mcast_addr", mcastAddr)
	return ai, nil
}

func normalizeScope(scope string) string {
	switch scope {
	case "link", "2":
		return ScopeLink
	case "admin", "4":
		return ScopeAdmin
	case "site", "5":
		return ScopeSite
	case "organisation", "organization", "8":
		return ScopeOrganisation
	case "global", "e":
		return ScopeGlobal
	default:
		return ScopeLink
	}
}

func normalizeMulticastType(mtype string) string {
	switch mtype {
	case "permanent", "0":
		return McastAddrTypePermanent
	case "temporary", "1":
		return McastAddrTypeTemporary
	default:
		return McastAddrTypeTemporary
	}
}

func (ai *AutoInterface) Start() error {
	ai.Mutex.Lock()
	// Only recreate done if it's nil or was closed
	select {
	case <-ai.done:
		ai.done = make(chan struct{})
		ai.stopOnce = sync.Once{}
	default:
		if ai.done == nil {
			ai.done = make(chan struct{})
			ai.stopOnce = sync.Once{}
		}
	}
	ai.Mutex.Unlock()

	interfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("failed to list interfaces: %v", err)
	}

	for _, iface := range interfaces {
		if ai.shouldIgnoreInterface(iface.Name) {
			debug.Log(debug.DebugTrace, "Ignoring interface", "name", iface.Name)
			continue
		}

		if len(ai.allowedInterfaces) > 0 && !ai.isAllowedInterface(iface.Name) {
			debug.Log(debug.DebugTrace, "Interface not in allowed list", "name", iface.Name)
			continue
		}

		ifaceCopy := iface
		if err := ai.configureInterface(&ifaceCopy); err != nil {
			debug.Log(debug.DebugVerbose, "Failed to configure interface", "name", iface.Name, "error", err)
			continue
		}
	}

	if len(ai.adoptedInterfaces) == 0 {
		return fmt.Errorf("no suitable interfaces found")
	}

	ai.Online = true
	ai.In = true
	ai.Out = true

	go ai.peerJobs()
	go ai.announceLoop()

	debug.Log(debug.DebugInfo, "AutoInterface started", "adopted", len(ai.adoptedInterfaces))
	return nil
}

func (ai *AutoInterface) shouldIgnoreInterface(name string) bool {
	ignoreList := []string{"lo", "lo0", "tun0", "awdl0", "llw0", "en5", "dummy0"}

	if slices.Contains(ai.ignoredInterfaces, name) {
		return true
	}

	return slices.Contains(ignoreList, name)
}

func (ai *AutoInterface) isAllowedInterface(name string) bool {
	return slices.Contains(ai.allowedInterfaces, name)
}

func (ai *AutoInterface) configureInterface(iface *net.Interface) error {
	if iface.Flags&net.FlagUp == 0 {
		return fmt.Errorf("interface is down")
	}

	if iface.Flags&net.FlagLoopback != 0 {
		return fmt.Errorf("loopback interface")
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return err
	}

	var linkLocalAddr string
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipnet.IP.To4() == nil && ipnet.IP.IsLinkLocalUnicast() {
				linkLocalAddr = descopeLinkLocal(ipnet.IP.String())
				break
			}
		}
	}

	if linkLocalAddr == "" {
		return fmt.Errorf("no link-local IPv6 address found")
	}

	ai.Mutex.Lock()
	ai.adoptedInterfaces[iface.Name] = &AdoptedInterface{
		name:          iface.Name,
		linkLocalAddr: linkLocalAddr,
		index:         iface.Index,
	}
	ai.linkLocalAddrs = append(ai.linkLocalAddrs, linkLocalAddr)
	ai.multicastEchoes[iface.Name] = time.Now()
	ai.Mutex.Unlock()

	if err := ai.startDiscoveryListener(iface); err != nil {
		return fmt.Errorf("failed to start discovery listener: %v", err)
	}

	if err := ai.startDataListener(iface); err != nil {
		return fmt.Errorf("failed to start data listener: %v", err)
	}

	debug.Log(debug.DebugInfo, "Configured interface", "name", iface.Name, "addr", linkLocalAddr)
	return nil
}

func (ai *AutoInterface) startDiscoveryListener(iface *net.Interface) error {
	addr := &net.UDPAddr{
		IP:   net.ParseIP(ai.mcastDiscoveryAddr),
		Port: ai.discoveryPort,
		Zone: iface.Name,
	}

	conn, err := net.ListenMulticastUDP("udp6", iface, addr)
	if err != nil {
		return err
	}

	if err := conn.SetReadBuffer(1024); err != nil {
		debug.Log(debug.DebugError, "Failed to set discovery read buffer", "error", err)
	}

	ai.Mutex.Lock()
	ai.discoveryServers[iface.Name] = conn
	ai.Mutex.Unlock()

	go ai.handleDiscovery(conn, iface.Name)
	debug.Log(debug.DebugVerbose, "Discovery listener started", "interface", iface.Name, "addr", ai.mcastDiscoveryAddr)
	return nil
}

func (ai *AutoInterface) startDataListener(iface *net.Interface) error {
	adoptedIface, exists := ai.adoptedInterfaces[iface.Name]
	if !exists {
		return fmt.Errorf("interface not adopted")
	}

	addr := &net.UDPAddr{
		IP:   net.ParseIP(adoptedIface.linkLocalAddr),
		Port: ai.dataPort,
		Zone: iface.Name,
	}

	conn, err := net.ListenUDP("udp6", addr)
	if err != nil {
		debug.Log(debug.DebugError, "Failed to listen on data port", "addr", addr, "error", err)
		return err
	}

	if err := conn.SetReadBuffer(ai.MTU); err != nil {
		debug.Log(debug.DebugError, "Failed to set data read buffer", "error", err)
	}

	ai.Mutex.Lock()
	ai.interfaceServers[iface.Name] = conn
	ai.Mutex.Unlock()

	go ai.handleData(conn, iface.Name)
	debug.Log(debug.DebugVerbose, "Data listener started", "interface", iface.Name, "addr", addr)
	return nil
}

func (ai *AutoInterface) handleDiscovery(conn *net.UDPConn, ifaceName string) {
	buf := make([]byte, 1024)
	for {
		ai.Mutex.RLock()
		done := ai.done
		ai.Mutex.RUnlock()

		select {
		case <-done:
			return
		default:
		}

		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ai.IsOnline() {
				debug.Log(debug.DebugError, "Discovery read error", "interface", ifaceName, "error", err)
			}
			return
		}

		peerIP := descopeLinkLocal(remoteAddr.IP.String())
		tokenSource := append(ai.groupID, []byte(peerIP)...)
		expectedHash := sha256.Sum256(tokenSource)

		if n >= len(expectedHash) {
			receivedHash := buf[:len(expectedHash)]
			if bytes.Equal(receivedHash, expectedHash[:]) {
				ai.handlePeerAnnounce(remoteAddr, ifaceName)
			} else {
				debug.Log(debug.DebugTrace, "Received discovery with mismatched group hash", "interface", ifaceName, "peer", peerIP)
			}
		}
	}
}

func (ai *AutoInterface) handleData(conn *net.UDPConn, ifaceName string) {
	buf := make([]byte, ai.GetMTU())
	for {
		ai.Mutex.RLock()
		done := ai.done
		ai.Mutex.RUnlock()

		select {
		case <-done:
			return
		default:
		}

		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ai.IsOnline() {
				debug.Log(debug.DebugError, "Data read error", "interface", ifaceName, "error", err)
			}
			return
		}

		data := buf[:n]
		dataHash := sha256.Sum256(data)
		now := time.Now()

		ai.Mutex.Lock()
		// Check for duplicate in mifDeque
		isDuplicate := false
		for i := 0; i < len(ai.mifDeque); i++ {
			if ai.mifDeque[i].hash == dataHash && now.Sub(ai.mifDeque[i].timestamp) < MultiIFDequeTTL {
				isDuplicate = true
				break
			}
		}

		if isDuplicate {
			ai.Mutex.Unlock()
			continue
		}

		// Add to deque
		ai.mifDeque = append(ai.mifDeque, DequeEntry{hash: dataHash, timestamp: now})
		if len(ai.mifDeque) > MultiIFDequeLen {
			ai.mifDeque = ai.mifDeque[1:]
		}

		// Refresh peer if known
		peerIP := descopeLinkLocal(remoteAddr.IP.String())
		peerKey := peerIP + "%" + ifaceName
		if peer, exists := ai.peers[peerKey]; exists {
			peer.lastHeard = now
		}
		ai.Mutex.Unlock()

		stripped, ok := common.ApplyIFACInbound(ai, data)
		if !ok {
			continue
		}
		if callback := ai.GetPacketCallback(); callback != nil {
			callback(stripped, ai)
		}
	}
}

func (ai *AutoInterface) handlePeerAnnounce(addr *net.UDPAddr, ifaceName string) {
	ai.Mutex.Lock()
	defer ai.Mutex.Unlock()

	peerIP := addr.IP.String()

	if slices.Contains(ai.linkLocalAddrs, peerIP) {
		ai.multicastEchoes[ifaceName] = time.Now()
		debug.Log(debug.DebugTrace, "Received own multicast echo", "interface", ifaceName)
		return
	}

	peerKey := peerIP + "%" + ifaceName

	if peer, exists := ai.peers[peerKey]; exists {
		peer.lastHeard = time.Now()
		debug.Log(debug.DebugTrace, "Updated peer", "peer", peerIP, "interface", ifaceName)
	} else {
		ai.peers[peerKey] = &Peer{
			ifaceName: ifaceName,
			lastHeard: time.Now(),
			addr:      addr,
		}
		debug.Log(debug.DebugInfo, "Discovered new peer", "peer", peerIP, "interface", ifaceName)
	}
}

func (ai *AutoInterface) announceLoop() {
	ticker := time.NewTicker(ai.announceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !ai.IsOnline() {
				return
			}
			ai.sendPeerAnnounce()
		case <-ai.done:
			return
		}
	}
}

func (ai *AutoInterface) sendPeerAnnounce() {
	ai.Mutex.RLock()
	defer ai.Mutex.RUnlock()

	for ifaceName, adoptedIface := range ai.adoptedInterfaces {
		mcastAddr := &net.UDPAddr{
			IP:   net.ParseIP(ai.mcastDiscoveryAddr),
			Port: ai.discoveryPort,
			Zone: ifaceName,
		}

		if ai.outboundConn == nil {
			var err error
			ai.outboundConn, err = net.ListenUDP("udp6", &net.UDPAddr{Port: 0})
			if err != nil {
				debug.Log(debug.DebugError, "Failed to create outbound socket", "error", err)
				return
			}
		}

		tokenSource := append(ai.groupID, []byte(adoptedIface.linkLocalAddr)...)
		token := sha256.Sum256(tokenSource)

		if _, err := ai.outboundConn.WriteToUDP(token[:], mcastAddr); err != nil {
			debug.Log(debug.DebugVerbose, "Failed to send peer announce", "interface", ifaceName, "error", err)
		} else {
			debug.Log(debug.DebugTrace, "Sent peer announce", "interface", adoptedIface.name)
		}
	}
}

func (ai *AutoInterface) peerJobs() {
	ticker := time.NewTicker(ai.peerJobInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !ai.IsOnline() {
				return
			}

			ai.Mutex.Lock()
			now := time.Now()

			for peerKey, peer := range ai.peers {
				if now.Sub(peer.lastHeard) > ai.peeringTimeout {
					delete(ai.peers, peerKey)
					debug.Log(debug.DebugVerbose, "Removed timed out peer", "peer", peerKey)
				}
			}

			for ifaceName, echoTime := range ai.multicastEchoes {
				if now.Sub(echoTime) > ai.mcastEchoTimeout {
					if _, exists := ai.timedOutInterfaces[ifaceName]; !exists {
						debug.Log(debug.DebugInfo, "Interface timed out", "interface", ifaceName)
						ai.timedOutInterfaces[ifaceName] = now
					}
				} else {
					delete(ai.timedOutInterfaces, ifaceName)
				}
			}

			ai.Mutex.Unlock()
		case <-ai.done:
			return
		}
	}
}

func (ai *AutoInterface) Send(data []byte, address string) error {
	if !ai.IsOnline() {
		return fmt.Errorf("interface offline")
	}

	masked, err := common.ApplyIFACOutbound(ai, data)
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to mask outgoing packet for IFAC", "name", ai.Name, "error", err)
		return err
	}
	data = masked

	ai.Mutex.RLock()
	defer ai.Mutex.RUnlock()

	if len(ai.peers) == 0 {
		debug.Log(debug.DebugTrace, "No peers available for sending")
		return nil
	}

	if ai.outboundConn == nil {
		var err error
		ai.outboundConn, err = net.ListenUDP("udp6", &net.UDPAddr{Port: 0})
		if err != nil {
			return fmt.Errorf("failed to create outbound socket: %v", err)
		}
	}

	sentCount := 0
	for _, peer := range ai.peers {
		targetAddr := &net.UDPAddr{
			IP:   peer.addr.IP,
			Port: ai.dataPort,
			Zone: peer.ifaceName,
		}

		if _, err := ai.outboundConn.WriteToUDP(data, targetAddr); err != nil {
			debug.Log(debug.DebugVerbose, "Failed to send to peer", "interface", peer.ifaceName, "error", err)
			continue
		}
		sentCount++
	}

	if sentCount > 0 {
		debug.Log(debug.DebugTrace, "Sent data to peers", "count", sentCount, "bytes", len(data))
	}

	return nil
}

func (ai *AutoInterface) Stop() error {
	ai.Mutex.Lock()
	ai.Online = false
	ai.In = false
	ai.Out = false

	for _, server := range ai.interfaceServers {
		server.Close() // #nosec G104
	}

	for _, server := range ai.discoveryServers {
		server.Close() // #nosec G104
	}

	if ai.outboundConn != nil {
		ai.outboundConn.Close() // #nosec G104
	}
	ai.Mutex.Unlock()

	ai.stopOnce.Do(func() {
		if ai.done != nil {
			close(ai.done)
		}
	})

	debug.Log(debug.DebugInfo, "AutoInterface stopped")
	return nil
}
