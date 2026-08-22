// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !rns_slim

package interfaces

import "testing"

func TestMatchesDiscoveredEndpointI2PPeer(t *testing.T) {
	eh := []byte{0x01, 0x02}
	peer := &I2PInterfacePeer{
		targetDest:      "peer.b32.i2p",
		AutoconnectHash: eh,
	}
	if !MatchesDiscoveredEndpoint(peer, eh, "peer.b32.i2p", 0, false) {
		t.Fatal("expected i2p peer match")
	}
	if MatchesDiscoveredEndpoint(peer, []byte{0xff}, "other.b32.i2p", 0, false) {
		t.Fatal("expected no match")
	}
}
