// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package interfaces

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net"
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
	// Python: re.sub(r"fe80:[0-9a-f]*::","fe80::", link_local_addr)
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
	groupID := DEFAULT_GROUP_ID
	if config.GroupID != "" {
		groupID = config.GroupID
	}

	discoveryScope := SCOPE_LINK
	if config.DiscoveryScope != "" {
		discoveryScope = normalizeScope(config.DiscoveryScope)
	}

	multicastAddrType := MCAST_ADDR_TYPE_TEMPORARY
	if config.MulticastAddrType != "" {
		multicastAddrType = normalizeMulticastType(config.MulticastAddrType)
	}

	discoveryPort := DEFAULT_DISCOVERY_PORT
	if config.DiscoveryPort != 0 {
		discoveryPort = config.DiscoveryPort
	}

	dataPort := DEFAULT_DATA_PORT
	if config.DataPort != 0 {
		dataPort = config.DataPort
	}

	groupHash := sha256.Sum256([]byte(groupID))

	// Python-compatible multicast address generation
	// gt = "0:" + "{:02x}".format(g[3]+(g[2]<<8)) + ":" + ...
	gt := "0"
	for i := 1; i <= 6; i++ {
		gt += fmt.Sprintf(":%02x%02x", groupHash[i*2], groupHash[i*2+1])
	}
	mcastAddr := fmt.Sprintf("ff%s%s:%s", multicastAddrType, discoveryScope, gt)

	ai := &AutoInterface{
		BaseInterface: BaseInterface{
			Name:     name,
			Mode:     common.IF_MODE_FULL,
			Type:     common.IF_TYPE_AUTO,
			Online:   false,
			Enabled:  config.Enabled,
			Detached: false,
			IN:       true,
			OUT:      false,
			MTU:      HW_MTU,
			Bitrate:  BITRATE_GUESS,
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
		announceInterval:   ANNOUNCE_INTERVAL,
		peerJobInterval:    PEER_JOB_INTERVAL,
		peeringTimeout:     PEERING_TIMEOUT,
		mcastEchoTimeout:   MCAST_ECHO_TIMEOUT,
		mifDeque:           make([]DequeEntry, 0, MULTI_IF_DEQUE_LEN),
		done:               make(chan struct{}),
	}

	debug.Log(debug.DEBUG_INFO, "AutoInterface configured", "name", name, "group", groupID, "mcast_addr", mcastAddr)
	return ai, nil
}

func normalizeScope(scope string) string {
	switch scope {
	case "link", "2":
		return SCOPE_LINK
	case "admin", "4":
		return SCOPE_ADMIN
	case "site", "5":
		return SCOPE_SITE
	case "organisation", "organization", "8":
		return SCOPE_ORGANISATION
	case "global", "e":
		return SCOPE_GLOBAL
	default:
		return SCOPE_LINK
	}
}

func normalizeMulticastType(mtype string) string {
	switch mtype {
	case "permanent", "0":
		return MCAST_ADDR_TYPE_PERMANENT
	case "temporary", "1":
		return MCAST_ADDR_TYPE_TEMPORARY
	default:
		return MCAST_ADDR_TYPE_TEMPORARY
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
			debug.Log(debug.DEBUG_TRACE, "Ignoring interface", "name", iface.Name)
			continue
		}

		if len(ai.allowedInterfaces) > 0 && !ai.isAllowedInterface(iface.Name) {
			debug.Log(debug.DEBUG_TRACE, "Interface not in allowed list", "name", iface.Name)
			continue
		}

		ifaceCopy := iface
		// bearer:disable go_gosec_memory_memory_aliasing
		if err := ai.configureInterface(&ifaceCopy); err != nil {
			debug.Log(debug.DEBUG_VERBOSE, "Failed to configure interface", "name", iface.Name, "error", err)
			continue
		}
	}

	if len(ai.adoptedInterfaces) == 0 {
		return fmt.Errorf("no suitable interfaces found")
	}

	ai.Online = true
	ai.IN = true
	ai.OUT = true

	go ai.peerJobs()
	go ai.announceLoop()

	debug.Log(debug.DEBUG_INFO, "AutoInterface started", "adopted", len(ai.adoptedInterfaces))
	return nil
}

func (ai *AutoInterface) shouldIgnoreInterface(name string) bool {
	ignoreList := []string{"lo", "lo0", "tun0", "awdl0", "llw0", "en5", "dummy0"}

	for _, ignored := range ai.ignoredInterfaces {
		if name == ignored {
			return true
		}
	}

	for _, ignored := range ignoreList {
		if name == ignored {
			return true
		}
	}

	return false
}

func (ai *AutoInterface) isAllowedInterface(name string) bool {
	for _, allowed := range ai.allowedInterfaces {
		if name == allowed {
			return true
		}
	}
	return false
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

	debug.Log(debug.DEBUG_INFO, "Configured interface", "name", iface.Name, "addr", linkLocalAddr)
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

	if err := conn.SetReadBuffer(common.NUM_1024); err != nil {
		debug.Log(debug.DEBUG_ERROR, "Failed to set discovery read buffer", "error", err)
	}

	ai.Mutex.Lock()
	ai.discoveryServers[iface.Name] = conn
	ai.Mutex.Unlock()

	go ai.handleDiscovery(conn, iface.Name)
	debug.Log(debug.DEBUG_VERBOSE, "Discovery listener started", "interface", iface.Name, "addr", ai.mcastDiscoveryAddr)
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
		debug.Log(debug.DEBUG_ERROR, "Failed to listen on data port", "addr", addr, "error", err)
		return err
	}

	if err := conn.SetReadBuffer(ai.MTU); err != nil {
		debug.Log(debug.DEBUG_ERROR, "Failed to set data read buffer", "error", err)
	}

	ai.Mutex.Lock()
	ai.interfaceServers[iface.Name] = conn
	ai.Mutex.Unlock()

	go ai.handleData(conn, iface.Name)
	debug.Log(debug.DEBUG_VERBOSE, "Data listener started", "interface", iface.Name, "addr", addr)
	return nil
}

func (ai *AutoInterface) handleDiscovery(conn *net.UDPConn, ifaceName string) {
	buf := make([]byte, common.NUM_1024)
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
				debug.Log(debug.DEBUG_ERROR, "Discovery read error", "interface", ifaceName, "error", err)
			}
			return
		}

		// Python: discovery_token = RNS.Identity.full_hash(self.group_id+ipv6_src[0].encode("utf-8"))
		peerIP := descopeLinkLocal(remoteAddr.IP.String())
		tokenSource := append(ai.groupID, []byte(peerIP)...)
		expectedHash := sha256.Sum256(tokenSource)

		if n >= len(expectedHash) {
			receivedHash := buf[:len(expectedHash)]
			if bytes.Equal(receivedHash, expectedHash[:]) {
				ai.handlePeerAnnounce(remoteAddr, ifaceName)
			} else {
				debug.Log(debug.DEBUG_TRACE, "Received discovery with mismatched group hash", "interface", ifaceName, "peer", peerIP)
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
				debug.Log(debug.DEBUG_ERROR, "Data read error", "interface", ifaceName, "error", err)
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
			if ai.mifDeque[i].hash == dataHash && now.Sub(ai.mifDeque[i].timestamp) < MULTI_IF_DEQUE_TTL {
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
		if len(ai.mifDeque) > MULTI_IF_DEQUE_LEN {
			ai.mifDeque = ai.mifDeque[1:]
		}

		// Refresh peer if known
		peerIP := descopeLinkLocal(remoteAddr.IP.String())
		peerKey := peerIP + "%" + ifaceName
		if peer, exists := ai.peers[peerKey]; exists {
			peer.lastHeard = now
		}
		ai.Mutex.Unlock()

		if callback := ai.GetPacketCallback(); callback != nil {
			callback(data, ai)
		}
	}
}

func (ai *AutoInterface) handlePeerAnnounce(addr *net.UDPAddr, ifaceName string) {
	ai.Mutex.Lock()
	defer ai.Mutex.Unlock()

	peerIP := addr.IP.String()

	for _, localAddr := range ai.linkLocalAddrs {
		if peerIP == localAddr {
			ai.multicastEchoes[ifaceName] = time.Now()
			debug.Log(debug.DEBUG_TRACE, "Received own multicast echo", "interface", ifaceName)
			return
		}
	}

	peerKey := peerIP + "%" + ifaceName

	if peer, exists := ai.peers[peerKey]; exists {
		peer.lastHeard = time.Now()
		debug.Log(debug.DEBUG_TRACE, "Updated peer", "peer", peerIP, "interface", ifaceName)
	} else {
		ai.peers[peerKey] = &Peer{
			ifaceName: ifaceName,
			lastHeard: time.Now(),
			addr:      addr,
		}
		debug.Log(debug.DEBUG_INFO, "Discovered new peer", "peer", peerIP, "interface", ifaceName)
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
				debug.Log(debug.DEBUG_ERROR, "Failed to create outbound socket", "error", err)
				return
			}
		}

		// Python: discovery_token = RNS.Identity.full_hash(self.group_id+link_local_address.encode("utf-8"))
		tokenSource := append(ai.groupID, []byte(adoptedIface.linkLocalAddr)...)
		token := sha256.Sum256(tokenSource)

		if _, err := ai.outboundConn.WriteToUDP(token[:], mcastAddr); err != nil {
			debug.Log(debug.DEBUG_VERBOSE, "Failed to send peer announce", "interface", ifaceName, "error", err)
		} else {
			debug.Log(debug.DEBUG_TRACE, "Sent peer announce", "interface", adoptedIface.name)
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
					debug.Log(debug.DEBUG_VERBOSE, "Removed timed out peer", "peer", peerKey)
				}
			}

			for ifaceName, echoTime := range ai.multicastEchoes {
				if now.Sub(echoTime) > ai.mcastEchoTimeout {
					if _, exists := ai.timedOutInterfaces[ifaceName]; !exists {
						debug.Log(debug.DEBUG_INFO, "Interface timed out", "interface", ifaceName)
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

	ai.Mutex.RLock()
	defer ai.Mutex.RUnlock()

	if len(ai.peers) == 0 {
		debug.Log(debug.DEBUG_TRACE, "No peers available for sending")
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
			debug.Log(debug.DEBUG_VERBOSE, "Failed to send to peer", "interface", peer.ifaceName, "error", err)
			continue
		}
		sentCount++
	}

	if sentCount > 0 {
		debug.Log(debug.DEBUG_TRACE, "Sent data to peers", "count", sentCount, "bytes", len(data))
	}

	return nil
}

func (ai *AutoInterface) Stop() error {
	ai.Mutex.Lock()
	ai.Online = false
	ai.IN = false
	ai.OUT = false

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

	debug.Log(debug.DEBUG_INFO, "AutoInterface stopped")
	return nil
}
