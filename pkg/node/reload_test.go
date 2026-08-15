// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/link"
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
	if !interfaceConfigsEqualForReload(nil, nil) {
		t.Fatal("nil nil")
	}
	if interfaceConfigsEqualForReload(a, nil) || interfaceConfigsEqualForReload(nil, a) {
		t.Fatal("nil vs non-nil")
	}

	c := &common.InterfaceConfig{Type: "UDPInterface", Address: ":0", Enabled: true, MTU: 500, Bitrate: 1000, PreferIPv6: true}
	d := &common.InterfaceConfig{Type: "UDPInterface", Address: ":0", Enabled: true, MTU: 500, Bitrate: 1000, PreferIPv6: true}
	if !interfaceConfigsEqualForReload(c, d) {
		t.Fatal("expected equal with MTU/bitrate/prefer_ipv6")
	}
	d.MTU = 1000
	if interfaceConfigsEqualForReload(c, d) {
		t.Fatal("expected not equal after MTU change")
	}
	d.MTU = 500
	d.Bitrate = 2000
	if interfaceConfigsEqualForReload(c, d) {
		t.Fatal("expected not equal after bitrate change")
	}
	d.Bitrate = 1000
	d.PreferIPv6 = false
	if interfaceConfigsEqualForReload(c, d) {
		t.Fatal("expected not equal after prefer_ipv6 change")
	}
	d.PreferIPv6 = true
	d.AnnounceCap = 5
	if interfaceConfigsEqualForReload(c, d) {
		t.Fatal("expected not equal after announce_cap change")
	}
	d.AnnounceCap = 0
	d.IngressControl = true
	d.IngressControlSet = true
	if interfaceConfigsEqualForReload(c, d) {
		t.Fatal("expected not equal after ingress_control change")
	}
	d.IngressControl = false
	d.IngressControlSet = false
	d.Outgoing = false
	d.OutgoingSet = true
	if interfaceConfigsEqualForReload(c, d) {
		t.Fatal("expected not equal after outgoing change")
	}
}

func TestOnNetworkLostCancelsReconnects(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = n.Stop() }()

	n.EnableLinkAutoReconnect(LinkReconnectOptions{MaxAttempts: 0, Backoff: time.Millisecond})
	if err := n.OnNetworkLost(); err != nil {
		t.Fatal(err)
	}
	if !n.networkPaused {
		t.Fatal("expected paused")
	}
	if !link.GlobalPaused() {
		t.Fatal("expected global pause")
	}
}
