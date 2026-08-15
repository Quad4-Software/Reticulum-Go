// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import "testing"

func TestParseInterfaceMode(t *testing.T) {
	cases := []struct {
		in   string
		want InterfaceMode
	}{
		{"", IFModeFull},
		{"full", IFModeFull},
		{"gateway", IFModeGateway},
		{"gw", IFModeGateway},
		{"access_point", IFModeAccessPoint},
		{"ap", IFModeAccessPoint},
		{"roaming", IFModeRoaming},
		{"boundary", IFModeBoundary},
		{"ptp", IFModePoint},
		{"internal", IFModeInternal},
	}
	for _, c := range cases {
		if got := ParseInterfaceMode(c.in); got != c.want {
			t.Errorf("ParseInterfaceMode(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestInterfaceModeWireValues(t *testing.T) {
	if IFModeFull != 0x01 || IFModePoint != 0x02 || IFModeAccessPoint != 0x03 ||
		IFModeRoaming != 0x04 || IFModeBoundary != 0x05 || IFModeGateway != 0x06 ||
		IFModeInternal != 0x07 {
		t.Fatalf("mode constants must match wire MODE_* values")
	}
}

func TestModeDiscoversPaths(t *testing.T) {
	if !ModeDiscoversPaths(IFModeInternal) || !ModeDiscoversPaths(IFModeGateway) {
		t.Fatal("internal and gateway should discover")
	}
	if ModeDiscoversPaths(IFModeFull) {
		t.Fatal("full should not discover by default")
	}
}
