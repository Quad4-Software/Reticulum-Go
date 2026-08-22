// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !rns_slim

package interfaces

import "testing"

func TestAdversarialMatchesDiscoveredEndpointI2P(t *testing.T) {
	peer := &I2PInterfacePeer{targetDest: ""}
	if MatchesDiscoveredEndpoint(peer, nil, "peer.b32.i2p", 0, false) {
		t.Fatal("empty peer dest must not match")
	}
	if MatchesDiscoveredEndpoint(peer, []byte{0x01}, "other.b32.i2p", 0, false) {
		t.Fatal("dest mismatch must not match")
	}
}
