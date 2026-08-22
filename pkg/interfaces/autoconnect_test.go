// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"testing"
)

func TestMatchesDiscoveredEndpointBackbone(t *testing.T) {
	eh := []byte{1, 2, 3}
	bc := &BackboneClientInterface{
		targetAddr:      "192.0.2.1",
		targetPort:      4242,
		AutoconnectHash: eh,
	}
	if !MatchesDiscoveredEndpoint(bc, eh, "192.0.2.1", 4242, true) {
		t.Fatal("expected hash match")
	}
	if MatchesDiscoveredEndpoint(bc, []byte{9}, "192.0.2.2", 4242, true) {
		t.Fatal("expected no match")
	}
}

func TestMatchesDiscoveredEndpointTCPClient(t *testing.T) {
	eh := bytes.Repeat([]byte{0xab}, 32)
	tc := &TCPClientInterface{
		targetAddr:      "10.0.0.5",
		targetPort:      7777,
		AutoconnectHash: eh,
	}
	if !MatchesDiscoveredEndpoint(tc, eh, "10.0.0.5", 7777, true) {
		t.Fatal("expected tcp client match")
	}
}
