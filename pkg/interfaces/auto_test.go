// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestNewAutoInterface(t *testing.T) {
	t.Run("DefaultConfig", func(t *testing.T) {
		config := &common.InterfaceConfig{Enabled: true}
		ai, err := NewAutoInterface("autoDefault", config)
		if err != nil {
			t.Fatalf("NewAutoInterface failed with default config: %v", err)
		}
		if ai == nil {
			t.Fatal("NewAutoInterface returned nil with default config")
		}

		if ai.GetName() != "autoDefault" {
			t.Errorf("GetName() = %s; want autoDefault", ai.GetName())
		}
		if ai.GetType() != common.IFTypeAuto {
			t.Errorf("GetType() = %v; want %v", ai.GetType(), common.IFTypeAuto)
		}
		if ai.discoveryPort != DefaultDiscoveryPort {
			t.Errorf("discoveryPort = %d; want %d", ai.discoveryPort, DefaultDiscoveryPort)
		}
		if ai.dataPort != DefaultDataPort {
			t.Errorf("dataPort = %d; want %d", ai.dataPort, DefaultDataPort)
		}
		if string(ai.groupID) != "reticulum" {
			t.Errorf("groupID = %s; want reticulum", string(ai.groupID))
		}
		if ai.discoveryScope != ScopeLink {
			t.Errorf("discoveryScope = %s; want %s", ai.discoveryScope, ScopeLink)
		}
		if len(ai.peers) != 0 {
			t.Errorf("peers map not empty initially")
		}
		if ai.unicastDiscoveryPort != DefaultDiscoveryPort+1 {
			t.Errorf("unicastDiscoveryPort = %d; want %d", ai.unicastDiscoveryPort, DefaultDiscoveryPort+1)
		}
		if ai.reversePeeringInterval != time.Duration(float64(AnnounceInterval)*3.25) {
			t.Errorf("reversePeeringInterval = %v; want %v", ai.reversePeeringInterval, time.Duration(float64(AnnounceInterval)*3.25))
		}
	})

	t.Run("CustomConfig", func(t *testing.T) {
		config := &common.InterfaceConfig{
			Enabled:       true,
			DiscoveryPort: 12345,
			DataPort:      54321,
			GroupID:       "customGroup",
		}
		ai, err := NewAutoInterface("autoCustom", config)
		if err != nil {
			t.Fatalf("NewAutoInterface failed with custom config: %v", err)
		}
		if ai == nil {
			t.Fatal("NewAutoInterface returned nil with custom config")
		}

		if ai.discoveryPort != 12345 {
			t.Errorf("discoveryPort = %d; want 12345", ai.discoveryPort)
		}
		if ai.dataPort != 54321 {
			t.Errorf("dataPort = %d; want 54321", ai.dataPort)
		}
		if string(ai.groupID) != "customGroup" {
			t.Errorf("groupID = %s; want customGroup", string(ai.groupID))
		}
	})

	t.Run("DevicesConfig", func(t *testing.T) {
		config := &common.InterfaceConfig{
			Enabled:        true,
			Devices:        []string{"eth0", "eth1"},
			IgnoredDevices: []string{"wlan0"},
		}
		ai, err := NewAutoInterface("autoDevices", config)
		if err != nil {
			t.Fatalf("NewAutoInterface failed: %v", err)
		}
		if !slices.Equal(ai.allowedInterfaces, []string{"eth0", "eth1"}) {
			t.Errorf("allowedInterfaces = %v; want [eth0 eth1]", ai.allowedInterfaces)
		}
		if !slices.Equal(ai.ignoredInterfaces, []string{"wlan0"}) {
			t.Errorf("ignoredInterfaces = %v; want [wlan0]", ai.ignoredInterfaces)
		}
	})
}

func TestAutoInterfacePeerCount(t *testing.T) {
	config := &common.InterfaceConfig{Enabled: true}
	ai, err := newMockAutoInterface("autoCount", config)
	if err != nil {
		t.Fatalf("Failed to create mock interface: %v", err)
	}

	if ai.PeerCount() != 0 {
		t.Errorf("PeerCount() = %d; want 0", ai.PeerCount())
	}

	ai.Mutex.Lock()
	ai.peers["fe80::1%eth0"] = &Peer{
		ifaceName: "eth0",
		lastHeard: time.Now(),
		addr:      &net.UDPAddr{IP: net.ParseIP("fe80::1"), Zone: "eth0"},
	}
	ai.peers["fe80::2%eth0"] = &Peer{
		ifaceName: "eth0",
		lastHeard: time.Now(),
		addr:      &net.UDPAddr{IP: net.ParseIP("fe80::2"), Zone: "eth0"},
	}
	ai.Mutex.Unlock()

	if ai.PeerCount() != 2 {
		t.Errorf("PeerCount() = %d; want 2", ai.PeerCount())
	}
}

// mockAutoInterface embeds AutoInterface but overrides methods that start goroutines
type mockAutoInterface struct {
	*AutoInterface
}

func newMockAutoInterface(name string, config *common.InterfaceConfig) (*mockAutoInterface, error) {
	ai, err := NewAutoInterface(name, config)
	if err != nil {
		return nil, err
	}

	// Initialize maps that would normally be initialized in Start()
	ai.peers = make(map[string]*Peer)
	ai.linkLocalAddrs = make([]string, 0)
	ai.adoptedInterfaces = make(map[string]*AdoptedInterface)
	ai.interfaceServers = make(map[string]*net.UDPConn)
	ai.discoveryServers = make(map[string]*net.UDPConn)
	ai.multicastEchoes = make(map[string]time.Time)
	ai.timedOutInterfaces = make(map[string]time.Time)

	return &mockAutoInterface{AutoInterface: ai}, nil
}

func (m *mockAutoInterface) Start() error {
	// Don't start any goroutines
	return nil
}

func (m *mockAutoInterface) Stop() error {
	// Don't try to close connections that were never opened
	return nil
}

// mockHandlePeerAnnounce is a test-only method that doesn't handle its own locking
func (m *mockAutoInterface) mockHandlePeerAnnounce(addr *net.UDPAddr, ifaceName string) {
	peerAddr := addr.IP.String() + "%" + addr.Zone

	if slices.Contains(m.linkLocalAddrs, peerAddr) {
		m.multicastEchoes[ifaceName] = time.Now()
		return
	}

	if _, exists := m.peers[peerAddr]; !exists {
		m.peers[peerAddr] = &Peer{
			ifaceName: ifaceName,
			lastHeard: time.Now(),
		}
	} else {
		m.peers[peerAddr].lastHeard = time.Now()
	}
}

func TestAutoInterfacePeerManagement(t *testing.T) {
	// Use a shorter timeout for testing
	testTimeout := 100 * time.Millisecond

	config := &common.InterfaceConfig{Enabled: true}
	ai, err := newMockAutoInterface("autoPeerTest", config)
	if err != nil {
		t.Fatalf("Failed to create mock interface: %v", err)
	}

	// Create a done channel to signal goroutine cleanup
	done := make(chan struct{})

	// Start peer management with done channel
	go func() {
		ticker := time.NewTicker(testTimeout)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ai.Mutex.Lock()
				now := time.Now()
				for addr, peer := range ai.peers {
					if now.Sub(peer.lastHeard) > testTimeout {
						delete(ai.peers, addr)
					}
				}
				ai.Mutex.Unlock()
			case <-done:
				return
			}
		}
	}()

	// Ensure cleanup
	defer func() {
		close(done)
		ai.Stop()
	}()

	// Simulate receiving peer announces
	peer1AddrStr := "fe80::1%eth0"
	peer2AddrStr := "fe80::2%eth0"
	localAddrStr := "fe80::aaaa%eth0" // Simulate a local address

	peer1Addr := &net.UDPAddr{IP: net.ParseIP("fe80::1"), Zone: "eth0"}
	peer2Addr := &net.UDPAddr{IP: net.ParseIP("fe80::2"), Zone: "eth0"}
	localAddr := &net.UDPAddr{IP: net.ParseIP("fe80::aaaa"), Zone: "eth0"}

	ai.Mutex.Lock()
	ai.linkLocalAddrs = append(ai.linkLocalAddrs, localAddrStr)
	ai.Mutex.Unlock()

	t.Run("AddPeer1", func(t *testing.T) {
		ai.Mutex.Lock()
		ai.mockHandlePeerAnnounce(peer1Addr, "eth0")
		ai.Mutex.Unlock()

		// Give a small amount of time for the peer to be processed
		time.Sleep(10 * time.Millisecond)

		ai.Mutex.RLock()
		count := len(ai.peers)
		peer, exists := ai.peers[peer1AddrStr]
		var ifaceName string
		if exists {
			ifaceName = peer.ifaceName
		}
		ai.Mutex.RUnlock()

		if count != 1 {
			t.Fatalf("Expected 1 peer, got %d", count)
		}
		if !exists {
			t.Fatalf("Peer %s not found in map", peer1AddrStr)
		}
		if ifaceName != "eth0" {
			t.Errorf("Peer %s interface name = %s; want eth0", peer1AddrStr, ifaceName)
		}
	})

	t.Run("AddPeer2", func(t *testing.T) {
		ai.Mutex.Lock()
		ai.mockHandlePeerAnnounce(peer2Addr, "eth0")
		ai.Mutex.Unlock()

		// Give a small amount of time for the peer to be processed
		time.Sleep(10 * time.Millisecond)

		ai.Mutex.RLock()
		count := len(ai.peers)
		_, exists := ai.peers[peer2AddrStr]
		ai.Mutex.RUnlock()

		if count != 2 {
			t.Fatalf("Expected 2 peers, got %d", count)
		}
		if !exists {
			t.Fatalf("Peer %s not found in map", peer2AddrStr)
		}
	})

	t.Run("IgnoreLocalAnnounce", func(t *testing.T) {
		ai.Mutex.Lock()
		ai.mockHandlePeerAnnounce(localAddr, "eth0")
		ai.Mutex.Unlock()

		// Give a small amount of time for the peer to be processed
		time.Sleep(10 * time.Millisecond)

		ai.Mutex.RLock()
		count := len(ai.peers)
		ai.Mutex.RUnlock()

		if count != 2 {
			t.Fatalf("Expected 2 peers after local announce, got %d", count)
		}
	})

	t.Run("UpdatePeerTimestamp", func(t *testing.T) {
		ai.Mutex.RLock()
		peer, exists := ai.peers[peer1AddrStr]
		var initialTime time.Time
		if exists {
			initialTime = peer.lastHeard
		}
		ai.Mutex.RUnlock()

		if !exists {
			t.Fatalf("Peer %s not found before timestamp update", peer1AddrStr)
		}

		ai.Mutex.Lock()
		ai.mockHandlePeerAnnounce(peer1Addr, "eth0")
		ai.Mutex.Unlock()

		// Give a small amount of time for the peer to be processed
		time.Sleep(10 * time.Millisecond)

		ai.Mutex.RLock()
		peer, exists = ai.peers[peer1AddrStr]
		var updatedTime time.Time
		if exists {
			updatedTime = peer.lastHeard
		}
		ai.Mutex.RUnlock()

		if !exists {
			t.Fatalf("Peer %s not found after timestamp update", peer1AddrStr)
		}

		if !updatedTime.After(initialTime) {
			t.Errorf("Peer timestamp was not updated after receiving another announce")
		}
	})

	t.Run("PeerTimeout", func(t *testing.T) {
		// Wait for peer timeout
		time.Sleep(testTimeout * 2)

		ai.Mutex.RLock()
		count := len(ai.peers)
		ai.Mutex.RUnlock()

		if count != 0 {
			t.Errorf("Expected all peers to timeout, got %d peers", count)
		}
	})
}

// pythonAutoHash runs the Python reference hash generation for the given
// group_id and peer_ip and returns the hex-encoded hash.
func pythonAutoHash(t *testing.T, groupID, peerIP string) string {
	t.Helper()
	script := fmt.Sprintf(
		"import sys; sys.path.insert(0, 'reticulum-ref'); import RNS; "+
			"print(RNS.Identity.full_hash(%q.encode('utf-8') + %q.encode('utf-8')).hex())",
		groupID, peerIP,
	)
	cmd := exec.Command("python3", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("python hash failed: %v\noutput: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// pythonAutoMcast runs the Python reference multicast address generation for
// the given group_id, scope and addr_type.
func pythonAutoMcast(t *testing.T, groupID, scope, addrType string) string {
	t.Helper()
	script := fmt.Sprintf(
		"import sys; sys.path.insert(0, 'reticulum-ref'); import RNS; "+
			"g = RNS.Identity.full_hash(%q.encode('utf-8')); "+
			"gt = '0'; "+
			"gt += ':'+'{:02x}'.format(g[3]+(g[2]<<8)); "+
			"gt += ':'+'{:02x}'.format(g[5]+(g[4]<<8)); "+
			"gt += ':'+'{:02x}'.format(g[7]+(g[6]<<8)); "+
			"gt += ':'+'{:02x}'.format(g[9]+(g[8]<<8)); "+
			"gt += ':'+'{:02x}'.format(g[11]+(g[10]<<8)); "+
			"gt += ':'+'{:02x}'.format(g[13]+(g[12]<<8)); "+
			"print('ff'+ %q + %q + ':' + gt)",
		groupID, addrType, scope,
	)
	cmd := exec.Command("python3", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("python mcast failed: %v\noutput: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestAutoInteropDiscoveryHash verifies that Go and Python generate the same
// discovery authentication hash.
func TestAutoInteropDiscoveryHash(t *testing.T) {
	cases := []struct {
		groupID string
		peerIP  string
	}{
		{"reticulum", "fe80::1"},
		{"reticulum", "fe80::abcd:1234"},
		{"customGroup", "fe80::2"},
		{"", "fe80::1"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("group=%s_peer=%s", tc.groupID, tc.peerIP), func(t *testing.T) {
			effectiveGroup := tc.groupID
			if effectiveGroup == "" {
				effectiveGroup = DefaultGroupID
			}
			pyHash := pythonAutoHash(t, effectiveGroup, tc.peerIP)
			if pyHash == "" {
				t.Skip("Python reference not available")
			}

			tokenSource := append([]byte(effectiveGroup), []byte(tc.peerIP)...)
			goHash := fmt.Sprintf("%x", sha256Hash(tokenSource))

			if goHash != pyHash {
				t.Errorf("hash mismatch: go=%s py=%s", goHash, pyHash)
			}
		})
	}
}

// TestAutoInteropMcastAddress verifies that Go and Python generate the same
// multicast discovery address.
func TestAutoInteropMcastAddress(t *testing.T) {
	cases := []struct {
		groupID  string
		scope    string
		addrType string
	}{
		{"reticulum", "2", "1"},
		{"reticulum", "2", "0"},
		{"customGroup", "2", "1"},
		{"reticulum", "5", "1"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("group=%s_scope=%s_type=%s", tc.groupID, tc.scope, tc.addrType), func(t *testing.T) {
			pyMcast := pythonAutoMcast(t, tc.groupID, tc.scope, tc.addrType)
			if pyMcast == "" {
				t.Skip("Python reference not available")
			}

			config := &common.InterfaceConfig{
				Enabled:           true,
				GroupID:           tc.groupID,
				DiscoveryScope:    tc.scope,
				MulticastAddrType: tc.addrType,
			}
			ai, err := NewAutoInterface("test", config)
			if err != nil {
				t.Fatalf("NewAutoInterface failed: %v", err)
			}

			if ai.mcastDiscoveryAddr != pyMcast {
				t.Errorf("mcast address mismatch: go=%s py=%s", ai.mcastDiscoveryAddr, pyMcast)
			}
		})
	}
}

// sha256Hash is a thin helper so the test doesn't depend on the exact
// crypto/sha256 import pattern used by the implementation.
func sha256Hash(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func setupRegressionVeth(t *testing.T, base string) (string, func()) {
	t.Helper()
	name := base
	peer := base + "p"
	_ = exec.Command("ip", "link", "del", name).Run()
	out, err := exec.Command("ip", "link", "add", name, "type", "veth", "peer", "name", peer).CombinedOutput()
	if err != nil {
		t.Skipf("veth not available: %v\n%s", err, out)
	}
	exec.Command("ip", "link", "set", name, "up").Run()
	exec.Command("ip", "link", "set", peer, "up").Run()
	exec.Command("ip", "-6", "addr", "add", "fe80::1/64", "dev", name, "nodad").Run()
	time.Sleep(100 * time.Millisecond)
	return name, func() { _ = exec.Command("ip", "link", "del", name).Run() }
}

func TestSelectLinkLocalAddrPrefersBindable(t *testing.T) {
	ifaceName, cleanup := setupRegressionVeth(t, "rnsll0")
	defer cleanup()

	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		t.Fatalf("InterfaceByName: %v", err)
	}

	selected := selectLinkLocalAddr(iface, DefaultDiscoveryPort+1)
	if selected == "" {
		t.Fatal("expected a link-local address")
	}
	if !canBindLinkLocalUDP(iface, selected, DefaultDiscoveryPort+1) {
		t.Fatalf("selected address %q is not bindable", selected)
	}
	if selected != "fe80::1" {
		t.Fatalf("selected = %q; want fe80::1 when manual nodad address is bindable", selected)
	}
}

func TestAutoInterfaceConfiguresBindableLinkLocalOnVeth(t *testing.T) {
	ifaceName, cleanup := setupRegressionVeth(t, "rnsll1")
	defer cleanup()

	ai, err := NewAutoInterface("auto", &common.InterfaceConfig{
		Enabled: true,
		Devices: []string{ifaceName},
	})
	if err != nil {
		t.Fatalf("NewAutoInterface: %v", err)
	}
	if err := ai.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer ai.Stop()

	ai.Mutex.RLock()
	adopted, ok := ai.adoptedInterfaces[ifaceName]
	hasOutbound := ai.outboundConns[ifaceName] != nil
	ai.Mutex.RUnlock()

	if !ok {
		t.Fatal("interface not adopted")
	}
	if adopted.linkLocalAddr != "fe80::1" {
		t.Fatalf("linkLocalAddr = %q; want fe80::1", adopted.linkLocalAddr)
	}
	if !hasOutbound {
		t.Fatal("expected outbound socket for adopted interface")
	}
}

func TestAutoInterfaceRescanOffline(t *testing.T) {
	cfg := &common.InterfaceConfig{Enabled: true}
	ai, err := NewAutoInterface("auto", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := ai.RescanInterfaces(); err == nil {
		t.Fatal("expected error when offline")
	}
}

func TestAutoInterfaceWatchInterfacesFlag(t *testing.T) {
	cfg := &common.InterfaceConfig{Enabled: true}
	ai, err := NewAutoInterface("auto", cfg)
	if err != nil {
		t.Fatal(err)
	}
	ai.SetWatchInterfaces(true)
	ai.Mutex.Lock()
	if !ai.watchInterfaces {
		t.Fatal("watch flag not set")
	}
	ai.lastRescan = time.Now()
	ai.maybeRescanLocked(time.Now())
	ai.Mutex.Unlock()
}

func TestUpdateLinkLocalAddressesEmpty(t *testing.T) {
	ai, err := NewAutoInterface("auto", &common.InterfaceConfig{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	ai.updateLinkLocalAddresses()
}
