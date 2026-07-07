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
func (m *mockTunnelPeer) WantsTunnel() bool       { return m.wantsTunnel }
func (m *mockTunnelPeer) SetWantsTunnel(v bool)   { m.wantsTunnel = v }
func (m *mockTunnelPeer) TunnelID() []byte        { return append([]byte(nil), m.tunnelID...) }
func (m *mockTunnelPeer) SetTunnelID(id []byte)   { m.tunnelID = append([]byte(nil), id...) }
func (m *mockTunnelPeer) Send(data []byte, _ string) error {
	m.sent = append(m.sent, append([]byte(nil), data...))
	return nil
}
func (m *mockTunnelPeer) Start() error   { return nil }
func (m *mockTunnelPeer) Stop() error    { return nil }
func (m *mockTunnelPeer) Enable()        {}
func (m *mockTunnelPeer) Disable()       {}
func (m *mockTunnelPeer) Detach()        {}
func (m *mockTunnelPeer) GetConn() net.Conn { return nil }
func (m *mockTunnelPeer) GetMTU() int    { return common.DefaultMTU }
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
func (m *mockTunnelPeer) GetTxBytes() uint64    { return 0 }
func (m *mockTunnelPeer) GetRxBytes() uint64    { return 0 }
func (m *mockTunnelPeer) GetTxPackets() uint64  { return 0 }
func (m *mockTunnelPeer) GetRxPackets() uint64  { return 0 }
func (m *mockTunnelPeer) SetIFAC(common.IFAC)   {}
func (m *mockTunnelPeer) GetIFAC() common.IFAC  { return nil }
func (m *mockTunnelPeer) ReceivedPathRequest()  {}
func (m *mockTunnelPeer) SentPathRequest()      {}
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
