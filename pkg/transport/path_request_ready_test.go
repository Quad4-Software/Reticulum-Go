// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
)

type bitrateIface struct {
	interfaces.BaseInterface
	bitrate int
}

func (b *bitrateIface) GetBitrate() int { return b.bitrate }

func TestIfaceReadyForPathRequestOnlineAndBitrate(t *testing.T) {
	bi := &bitrateIface{}
	bi.BaseInterface = interfaces.NewBaseInterface("radio", common.IFTypeUDP, true)
	bi.Online = false
	bi.bitrate = 1200
	if ifaceReadyForPathRequest(bi) {
		t.Fatal("offline iface must not be ready")
	}
	bi.Online = true
	bi.bitrate = 0
	if ifaceReadyForPathRequest(bi) {
		t.Fatal("zero bitrate iface must not be ready")
	}
	bi.bitrate = 1200
	if !ifaceReadyForPathRequest(bi) {
		t.Fatal("online positive-bitrate iface should be ready")
	}
}
