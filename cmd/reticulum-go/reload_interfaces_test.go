package main

import (
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
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
