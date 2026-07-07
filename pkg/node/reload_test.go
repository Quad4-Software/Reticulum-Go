// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestInterfaceConfigsEqualForReload(t *testing.T) {
	a := &common.InterfaceConfig{Type: "UDPInterface", Address: ":0", TargetPort: 7, Enabled: true}
	b := &common.InterfaceConfig{Type: "UDPInterface", Address: ":0", TargetPort: 7, Enabled: true}
	if !interfaceConfigsEqualForReload(a, b) {
		t.Fatal("expected equal")
	}
	b.Address = ":1"
	if interfaceConfigsEqualForReload(a, b) {
		t.Fatal("expected not equal after address change")
	}
	if interfaceConfigsEqualForReload(nil, nil) != true {
		t.Fatal("nil nil")
	}
	if interfaceConfigsEqualForReload(a, nil) || interfaceConfigsEqualForReload(nil, a) {
		t.Fatal("nil vs non-nil")
	}
}
