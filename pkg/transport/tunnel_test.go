// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"net"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
)

type mockTunnelPeer struct {
	common.BaseInterface
	name        string
	wantsTunnel bool
	tunnelID    []byte
	sent        [][]byte
}

func (m *mockTunnelPeer) GetName() string { return m.name }
func (m *mockTunnelPeer) InterfaceHash() []byte {
	return interfaces.InterfaceHashFromName(m.name)
}
func (m *mockTunnelPeer) WantsTunnel() bool     { return m.wantsTunnel }
func (m *mockTunnelPeer) SetWantsTunnel(v bool) { m.wantsTunnel = v }
func (m *mockTunnelPeer) TunnelID() []byte      { return append([]byte(nil), m.tunnelID...) }
func (m *mockTunnelPeer) SetTunnelID(id []byte) { m.tunnelID = append([]byte(nil), id...) }
func (m *mockTunnelPeer) Send(data []byte, _ string) error {
	m.sent = append(m.sent, append([]byte(nil), data...))
	return nil
}
func (m *mockTunnelPeer) Start() error      { return nil }
func (m *mockTunnelPeer) Stop() error       { return nil }
func (m *mockTunnelPeer) Enable()           {}
func (m *mockTunnelPeer) Disable()          {}
func (m *mockTunnelPeer) Detach()           {}
func (m *mockTunnelPeer) GetConn() net.Conn { return nil }
func (m *mockTunnelPeer) GetMTU() int       { return common.DefaultMTU }
func (m *mockTunnelPeer) GetType() common.InterfaceType {
	return common.IFTypeI2P
}
func (m *mockTunnelPeer) GetMode() common.InterfaceMode { return common.IFModeFull }
func (m *mockTunnelPeer) IsEnabled() bool               { return true }
func (m *mockTunnelPeer) IsOnline() bool                { return true }
func (m *mockTunnelPeer) IsDetached() bool              { return false }
func (m *mockTunnelPeer) GetBandwidthAvailable() bool   { return true }
func (m *mockTunnelPeer) ProcessIncoming([]byte)        {}
func (m *mockTunnelPeer) ProcessOutgoing([]byte) error  { return nil }
func (m *mockTunnelPeer) SendPathRequest([]byte) error  { return nil }
func (m *mockTunnelPeer) SendLinkPacket([]byte, []byte, time.Time) error {
	return nil
}
func (m *mockTunnelPeer) SetPacketCallback(common.PacketCallback) {}
func (m *mockTunnelPeer) GetPacketCallback() common.PacketCallback {
	return nil
}
func (m *mockTunnelPeer) GetTxBytes() uint64         { return 0 }
func (m *mockTunnelPeer) GetRxBytes() uint64         { return 0 }
func (m *mockTunnelPeer) GetTxPackets() uint64       { return 0 }
func (m *mockTunnelPeer) GetRxPackets() uint64       { return 0 }
func (m *mockTunnelPeer) SetIFAC(common.IFAC)        {}
func (m *mockTunnelPeer) GetIFAC() common.IFAC       { return nil }
func (m *mockTunnelPeer) ReceivedPathRequest()       {}
func (m *mockTunnelPeer) SentPathRequest()           {}
func (m *mockTunnelPeer) ShouldIngressLimitPR() bool { return false }
func (m *mockTunnelPeer) ShouldEgressLimitPR() bool  { return false }
func (m *mockTunnelPeer) SetPRBurstConfig(float64, float64, float64, bool) {
}

func TestSynthesizeTunnelSendsBroadcast(t *testing.T) {
	tr := NewTransport(nil)
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatal(err)
	}
	peer := &mockTunnelPeer{name: "i2p0 to peer.b32.i2p", wantsTunnel: true}
	if err := tr.SynthesizeTunnel(peer); err != nil {
		t.Fatal(err)
	}
	if peer.wantsTunnel {
		t.Fatal("wantsTunnel should be cleared after synthesize")
	}
	if len(peer.sent) != 1 {
		t.Fatalf("expected 1 outbound packet, got %d", len(peer.sent))
	}
}

func TestTunnelTableSize(t *testing.T) {
	tr := &Transport{}
	ids := make([][]byte, maxTunnels)
	for i := range maxTunnels {
		peer := &mockTunnelPeer{name: "i2p0 to peer.b32.i2p"}
		tid := make([]byte, 32)
		tid[0] = byte(i)
		tid[1] = byte(i >> 8)
		ids[i] = tid
		tr.handleTunnel(tid, peer)
		if len(peer.tunnelID) != 32 {
			t.Fatalf("row %d was not stored", i)
		}
	}

	extra := &mockTunnelPeer{name: "i2p0 to extra.b32.i2p"}
	extraID := make([]byte, 32)
	extraID[2] = 0x01
	tr.handleTunnel(extraID, extra)
	if len(extra.tunnelID) != 0 {
		t.Fatal("table accepted a row past maxTunnels")
	}

	again := &mockTunnelPeer{name: "i2p0 to peer.b32.i2p"}
	tr.handleTunnel(ids[0], again)
	if len(again.tunnelID) != 32 {
		t.Fatal("existing row was not refreshed")
	}

	tr.tunnelMu.Lock()
	held := len(tr.tunnels)
	tr.tunnelMu.Unlock()
	if held != maxTunnels {
		t.Fatalf("held %d rows, want %d", held, maxTunnels)
	}
}

func TestTunnelEntryExpiry(t *testing.T) {
	tr := &Transport{}
	peer := &mockTunnelPeer{name: "i2p0 to peer.b32.i2p"}
	id := make([]byte, 32)
	id[0] = 0x42
	tr.handleTunnel(id, peer)
	if len(peer.tunnelID) != 32 {
		t.Fatal("row was not stored")
	}

	var key [32]byte
	copy(key[:], id)
	tr.tunnelMu.Lock()
	entry := tr.tunnels[key]
	if entry == nil {
		tr.tunnelMu.Unlock()
		t.Fatal("missing stored row")
	}
	remaining := time.Until(entry.expires)
	entry.expires = time.Now().Add(-time.Second)
	tr.tunnelMu.Unlock()
	if remaining < 7*time.Hour || remaining > 8*time.Hour+time.Minute {
		t.Fatalf("fresh row remaining %s, want about 8h", remaining)
	}

	next := &mockTunnelPeer{name: "i2p0 to next.b32.i2p"}
	nextID := make([]byte, 32)
	nextID[0] = 0x43
	tr.handleTunnel(nextID, next)
	if len(next.tunnelID) != 32 {
		t.Fatal("expired row did not free a slot")
	}

	tr.tunnelMu.Lock()
	_, oldHeld := tr.tunnels[key]
	held := len(tr.tunnels)
	tr.tunnelMu.Unlock()
	if oldHeld {
		t.Fatal("expired row was still present")
	}
	if held != 1 {
		t.Fatalf("held %d rows, want 1", held)
	}
}
