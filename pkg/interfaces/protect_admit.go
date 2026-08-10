// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/protect"
)

func ifaceBitrate(iface common.NetworkInterface) int64 {
	if iface == nil {
		return 0
	}
	switch br := iface.(type) {
	case interface{ GetBitrate() int64 }:
		return br.GetBitrate()
	case interface{ GetBitrate() int }:
		return int64(br.GetBitrate())
	case interface{ GetBitrate() uint64 }:
		return int64(br.GetBitrate()) // #nosec G115 -- rate hint only
	}
	return 0
}

func admitIncoming(iface common.NetworkInterface, name string, data []byte) bool {
	return admitIncomingFrom(iface, name, data, "")
}

// admitIncomingFrom is admitIncoming plus an optional peerKey identifying
// the remote sender on a shared local interface (for example a listener
// accepting many client connections, or a single UDP socket serving many
// remote peers). Passing a peerKey gives that sender its own fair-share
// sub-bucket so it cannot exhaust the whole interface budget and cool down
// every other peer sharing it. Pass "" for interfaces that are inherently
// single-peer, where the interface bucket already is the peer bucket.
func admitIncomingFrom(iface common.NetworkInterface, name string, data []byte, peerKey string) bool {
	opts := protect.AdmitOpts{
		Bitrate: ifaceBitrate(iface),
		Class:   protect.PeekPacketClass(data),
		PeerKey: peerKey,
	}
	return protect.AdmitPacketOpts(name, len(data), opts).Allow
}
