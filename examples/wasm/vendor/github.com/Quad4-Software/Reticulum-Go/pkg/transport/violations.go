// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

func (t *Transport) applyPacketFilter(pkt *packet.Packet, iface common.NetworkInterface) bool {
	if t == nil || pkt == nil {
		return false
	}
	if t.packetFilter(pkt) {
		return true
	}
	if iface != nil {
		if h, ok := iface.(interface{ PacketFilterHit() }); ok {
			h.PacketFilterHit()
		}
	}
	return false
}

func ifaceProtocolViolation(iface common.NetworkInterface) {
	if iface == nil {
		return
	}
	if v, ok := iface.(interface{ ProtocolViolation() }); ok {
		v.ProtocolViolation()
	}
}

func ifaceIFACViolation(iface common.NetworkInterface) {
	if iface == nil {
		return
	}
	if v, ok := iface.(interface{ IFACViolation() }); ok {
		v.IFACViolation()
	}
}
