// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/packet"
)

const (
	lrECPubSize = 64
	lrMTUSize   = 3
)

// clampRelayedLinkRequestMTU mirrors Python transport LINKREQUEST MTU clamp
// against receiving and next-hop interface MTUs.
func clampRelayedLinkRequestMTU(raw []byte, pkt *packet.Packet, recv, out common.NetworkInterface) []byte {
	if pkt == nil || len(raw) < lrMTUSize || len(pkt.Data) < lrECPubSize+lrMTUSize {
		return raw
	}
	pathMTU, mode, ok := mtuFromLRData(pkt.Data)
	if !ok || pathMTU <= 0 {
		return raw
	}

	nhMTU := 0
	if out != nil {
		nhMTU = out.GetMTU()
	}
	phMTU := 0
	if recv != nil {
		phMTU = recv.GetMTU()
	}

	if nhMTU <= 0 {
		debug.Log(debug.DebugInfo, "No next-hop MTU, stripping link MTU signalling")
		return raw[:len(raw)-lrMTUSize]
	}

	limit := nhMTU
	if phMTU > 0 && phMTU < limit {
		limit = phMTU
	}
	if pathMTU <= limit {
		return raw
	}

	clamped := signallingBytesMTU(limit, mode)
	outRaw := append([]byte(nil), raw[:len(raw)-lrMTUSize]...)
	outRaw = append(outRaw, clamped...)
	debug.Log(debug.DebugInfo, "Clamping relayed link MTU",
		"requested", pathMTU, "clamped", limit)
	return outRaw
}

func mtuFromLRData(data []byte) (mtu int, mode byte, ok bool) {
	if len(data) < lrECPubSize+lrMTUSize {
		return 0, 0, false
	}
	b := data[lrECPubSize : lrECPubSize+lrMTUSize]
	mtu = (int(b[0]&0x1F) << 16) | (int(b[1]) << 8) | int(b[2])
	mode = (b[0] & 0xE0) >> 5
	return mtu, mode, true
}

func signallingBytesMTU(mtu int, mode byte) []byte {
	b := make([]byte, lrMTUSize)
	b[0] = byte((mtu >> 16) & 0xFF)
	b[1] = byte((mtu >> 8) & 0xFF)
	b[2] = byte(mtu & 0xFF)
	b[0] |= (mode << 5)
	return b
}
