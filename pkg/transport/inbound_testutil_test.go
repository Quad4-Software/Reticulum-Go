// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/protect"
)

func newOnlineTestIface(name string) *common.BaseInterface {
	iface := common.NewBaseInterfacePtr(name, common.IFTypeUDP, true)
	iface.Online = true
	return iface
}

// fillPacketQueueForTest blocks every handler worker so enqueuePacket fails
// until the hold channel is closed via t.Cleanup.
func fillPacketQueueForTest(t *testing.T, tr *Transport) {
	t.Helper()
	hold := make(chan struct{})
	if n := tr.occupyHandlerPoolForTest(hold); n == 0 {
		t.Fatal("handler pool not occupied")
	}
	t.Cleanup(func() { close(hold) })
}

func waitProtectHandlerTrip(t *testing.T, e *protect.Engine, tr *Transport, iface common.NetworkInterface, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	pkt := minimalDataPacket()
	for time.Now().Before(deadline) {
		if e.TripCount(protect.ReasonHandler) > 0 {
			return
		}
		tr.HandlePacket(pkt, iface)
		time.Sleep(time.Millisecond)
	}
	t.Fatal("expected handler trip")
}

func minimalDataPacket() []byte {
	dest := bytes.Repeat([]byte{0x02}, 16)
	flags := byte(0)
	flags |= (packet.HeaderType1 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.PropagationBroadcast << 4) & packet.HeaderMaskTransportType
	flags |= (packet.DestinationSingle << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeData & packet.HeaderMaskPacketType

	out := make([]byte, 0, 2+16+1+1)
	out = append(out, flags, 0x00)
	out = append(out, dest...)
	out = append(out, packet.ContextNone, 0xab)
	return out
}

func waitInboundDrain(t *testing.T, tr *Transport, extra time.Duration) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		total, _, _ := tr.inboundQueueSnapshot()
		if total == 0 {
			time.Sleep(extra)
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("inbound queue did not drain")
}
