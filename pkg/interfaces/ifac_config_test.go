// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/ifac"
)

func TestApplyIFACFromConfig(t *testing.T) {
	iface := &UDPInterface{
		BaseInterface: NewBaseInterface("test", common.IFTypeUDP, true),
	}
	cfg := &common.InterfaceConfig{
		Name:        "test",
		NetworkName: "test-net",
		Passphrase:  "secret",
		IFACSize:    16,
	}
	if err := ApplyIFACFromConfig(iface, cfg); err != nil {
		t.Fatalf("ApplyIFACFromConfig: %v", err)
	}
	id := iface.GetIFAC()
	if id == nil {
		t.Fatal("expected IFAC identity")
	}
	if id.Size() != 16 {
		t.Fatalf("size = %d, want 16", id.Size())
	}

	cfg2 := &common.InterfaceConfig{Name: "empty"}
	if err := ApplyIFACFromConfig(iface, cfg2); err != nil {
		t.Fatalf("empty config: %v", err)
	}

	cfg3 := &common.InterfaceConfig{
		Name:        "bits",
		IFACNetname: "n",
		IFACNetkey:  "k",
		IFACSize:    ifac.DefaultSize,
	}
	if err := ApplyIFACFromConfig(iface, cfg3); err != nil {
		t.Fatalf("explicit ifac keys: %v", err)
	}
}

func TestIFACSizeFromBits(t *testing.T) {
	tests := []struct {
		bits int
		want int
	}{
		{128, 16},
		{8, 1},
		{4, 0},
	}
	for _, tc := range tests {
		var size int
		if tc.bits >= ifac.MinSize*8 {
			size = tc.bits / 8
		}
		if size != tc.want {
			t.Errorf("bits %d => %d, want %d", tc.bits, size, tc.want)
		}
	}
}
