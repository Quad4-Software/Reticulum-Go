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
	opts := protect.AdmitOpts{
		Bitrate: ifaceBitrate(iface),
		Class:   protect.PeekPacketClass(data),
	}
	return protect.AdmitPacketOpts(name, len(data), opts).Allow
}
