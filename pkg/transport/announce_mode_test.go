// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
)

func TestShouldForwardAnnounceAccessPointBlocked(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	ap := &interfaces.UDPInterface{}
	ap.BaseInterface = interfaces.NewBaseInterface("ap", common.IFTypeUDP, true)
	ap.Mode = common.IFModeAccessPoint
	ap.Online = true
	dest := make([]byte, 16)
	dest[0] = 1
	if tr.shouldForwardAnnounceOn(dest, ap, ap) {
		t.Fatal("AP mode should block announce forward")
	}
}

func TestShouldForwardAnnounceFullAllowed(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	full := &interfaces.UDPInterface{}
	full.BaseInterface = interfaces.NewBaseInterface("full", common.IFTypeUDP, true)
	full.Mode = common.IFModeFull
	full.Online = true
	dest := make([]byte, 16)
	dest[0] = 2
	if !tr.shouldForwardAnnounceOn(dest, full, full) {
		t.Fatal("full mode with inbound iface should allow announce forward")
	}
}

func TestIfaceDiscoversUnknownPaths(t *testing.T) {
	bi := interfaces.NewBaseInterface("gw", common.IFTypeUDP, true)
	bi.Mode = common.IFModeGateway
	if !ifaceDiscoversUnknownPaths(&bi) {
		t.Fatal("gateway should discover paths")
	}
	bi.Mode = common.IFModeFull
	if ifaceDiscoversUnknownPaths(&bi) {
		t.Fatal("full mode should not discover unless recursive_prs")
	}
	bi.RecursivePRs = true
	if !ifaceDiscoversUnknownPaths(&bi) {
		t.Fatal("recursive_prs should discover paths")
	}
}

func TestParseInterfaceModeInternal(t *testing.T) {
	if common.ParseInterfaceMode("internal") != common.IFModeInternal {
		t.Fatal("expected MODE_INTERNAL 0x07")
	}
	if common.IFModeGateway != 0x06 {
		t.Fatalf("gateway wire value = %d, want 0x06", common.IFModeGateway)
	}
}
