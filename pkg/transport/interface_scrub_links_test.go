// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
)

// linkBoundIface is a minimal [LinkInterface] bound to a [common.NetworkInterface].
type linkBoundIface struct {
	id []byte
	ni common.NetworkInterface
}

func (l *linkBoundIface) GetStatus() byte { return 0 }
func (l *linkBoundIface) GetRTT() float64 { return 0 }
func (l *linkBoundIface) RTT() float64    { return 0 }
func (l *linkBoundIface) GetLinkID() []byte {
	if len(l.id) > 16 {
		return l.id[:16]
	}
	return l.id
}
func (l *linkBoundIface) Send(data []byte) any { return &packet.Packet{Raw: data} }
func (l *linkBoundIface) Resend(p any) error   { return nil }
func (l *linkBoundIface) SetPacketTimeout(p any, cb func(any), t time.Duration) {
}
func (l *linkBoundIface) SetPacketDelivered(p any, cb func(any)) {}
func (l *linkBoundIface) HandleInbound(pkt *packet.Packet) error { return nil }
func (l *linkBoundIface) ValidateLinkProof(pkt *packet.Packet, networkIface common.NetworkInterface) error {
	return nil
}
func (l *linkBoundIface) LinkedNetworkInterface() common.NetworkInterface { return l.ni }

func TestUnregisterInterfaceScrubsRegisteredLinks(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	iface := mockIface("wan", true)
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatal(err)
	}
	linkID := bytes.Repeat([]byte{0xEE}, 16)
	tr.RegisterLink(linkID, &linkBoundIface{id: linkID, ni: iface})

	tr.UnregisterInterface("wan")

	tr.mutex.RLock()
	_, exists := tr.links[hash16FromSlice(linkID)]
	tr.mutex.RUnlock()
	if exists {
		t.Fatal("link table should drop entries bound to removed interface")
	}
}
