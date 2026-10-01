// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"bytes"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/discovery"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
)

// qualifyInfo builds a TCPServerInterface announce carrying the given
// implementation and version strings.
func qualifyInfo(impl, vers string) *discovery.ReceivedAnnounceInfo {
	return &discovery.ReceivedAnnounceInfo{
		Info: discovery.Info{
			Type:          "TCPServerInterface",
			Name:          "qualify",
			ReachableOn:   "192.0.2.70",
			Port:          4242,
			HasPort:       true,
			Transport:     true,
			TransportImpl: impl,
			TransportVers: vers,
		},
		RemoteIdentity: bytes.Repeat([]byte{0x44}, 16),
	}
}

// TestAutoconnectQualifyMatrix covers the RNS 1.5.5 AUTOCONNECT_IMPLS /
// AUTOCONNECT_MIN_V criteria and the unverified-implementations bypass.
func TestAutoconnectQualifyMatrix(t *testing.T) {
	cases := []struct {
		name       string
		impl, vers string
		unverified bool
		want       bool
	}{
		{"rns at min", "RNS", "1.5.2", false, true},
		{"rns above min", "RNS", "1.5.5", false, true},
		{"rns next major", "RNS", "1.6.0", false, true},
		{"rns below min", "RNS", "1.5.1", false, false},
		{"rns old major", "RNS", "1.4.9", false, false},
		{"ret-go at min", "reticulum-go", "1.1.0", false, true},
		{"ret-go v-prefixed", "reticulum-go", "v1.2.0", false, true},
		{"ret-go below min", "reticulum-go", "1.0.9", false, false},
		{"unknown impl", "nomadnet", "3.0.0", false, false},
		{"missing impl", "", "1.5.5", false, false},
		{"missing version", "RNS", "", false, false},
		{"garbage version", "RNS", "not.a.version", false, false},
		{"unverified bypass", "", "", true, true},
		{"unverified unknown impl", "nomadnet", "0.0.1", true, true},
	}
	for _, tc := range cases {
		cfg := common.DefaultConfig()
		cfg.AutoconnectDiscoveredInterfaces = 4
		cfg.AutoconnectUnverifiedImplementations = tc.unverified
		n, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		info := qualifyInfo(tc.impl, tc.vers)
		got := n.autoconnectQualified(info)
		n.Stop()
		if got != tc.want {
			t.Fatalf("%s: qualified=%v want %v", tc.name, got, tc.want)
		}
	}
}

// TestAutoconnectQualifiedBlocksConnect verifies the gate actually stops the
// spawn path, not just the predicate.
func TestAutoconnectQualifiedBlocksConnect(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.AutoconnectDiscoveredInterfaces = 4
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()
	n.autoconnect(qualifyInfo("nomadnet", "9.9.9"))
	if n.autoconnectCount() != 0 {
		t.Fatal("unqualified impl must not autoconnect")
	}
	n.autoconnect(qualifyInfo("RNS", "1.5.1"))
	if n.autoconnectCount() != 0 {
		t.Fatal("below-min RNS version must not autoconnect")
	}
	n.autoconnect(qualifyInfo("RNS", "1.5.5"))
	if n.autoconnectCount() != 1 {
		t.Fatalf("qualified announce should connect, count=%d", n.autoconnectCount())
	}
}

// TestAutoconnectSequentialNaming drives autoconnectInterfaceName against
// interfaces already registered in transport (RNS 1.5.5 "Name (N)").
func TestAutoconnectSequentialNamingCollision(t *testing.T) {
	cfg := common.DefaultConfig()
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	base := "peer (10.0.0.9:4242)"
	for _, name := range []string{base, base + " (2)"} {
		u, err := interfaces.NewUDPInterface(name, "127.0.0.1:0", "", false)
		if err != nil {
			t.Fatal(err)
		}
		if err := n.transport.RegisterInterface(name, u); err != nil {
			t.Fatal(err)
		}
		defer n.transport.UnregisterInterface(name)
	}

	info := &discovery.ReceivedAnnounceInfo{Info: discovery.Info{
		Type: "TCPServerInterface", Name: "peer", ReachableOn: "10.0.0.9",
		Port: 4242, HasPort: true,
	}}
	if got := n.autoconnectInterfaceName(info); got != base+" (3)" {
		t.Fatalf("sequential name got %q want %q", got, base+" (3)")
	}
}
