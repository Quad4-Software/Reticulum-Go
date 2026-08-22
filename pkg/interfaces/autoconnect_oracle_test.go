// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"testing"
)

func TestOracleMatchesDiscoveredEndpointHashWinsOverHost(t *testing.T) {
	eh := bytes.Repeat([]byte{0x01}, 32)
	tc := &TCPClientInterface{
		targetAddr:      "10.0.0.9",
		targetPort:      1111,
		AutoconnectHash: eh,
	}
	if !MatchesDiscoveredEndpoint(tc, eh, "different.host", 9999, true) {
		t.Fatal("hash match should win even when host/port differ")
	}
}

func TestOracleMatchesDiscoveredEndpointHostPortWithoutHash(t *testing.T) {
	bc := &BackboneClientInterface{targetAddr: "203.0.113.5", targetPort: 8080}
	if !MatchesDiscoveredEndpoint(bc, nil, "203.0.113.5", 8080, true) {
		t.Fatal("host/port match expected")
	}
	if MatchesDiscoveredEndpoint(bc, nil, "203.0.113.5", 8081, true) {
		t.Fatal("port mismatch should not match")
	}
}

func TestOracleMatchesDiscoveredEndpointNoPortInAnnounce(t *testing.T) {
	tc := &TCPClientInterface{targetAddr: "198.51.100.10", targetPort: 4242}
	if !MatchesDiscoveredEndpoint(tc, nil, "198.51.100.10", 9999, false) {
		t.Fatal("missing announce port should match any client port on host")
	}
}

func TestAdversarialMatchesDiscoveredEndpointNilIface(t *testing.T) {
	if MatchesDiscoveredEndpoint(nil, []byte{1}, "host", 1, true) {
		t.Fatal("nil iface must not match")
	}
}

func TestAdversarialMatchesDiscoveredEndpointEmptyFields(t *testing.T) {
	tc := &TCPClientInterface{}
	if MatchesDiscoveredEndpoint(tc, nil, "", 0, false) {
		t.Fatal("empty endpoints must not match")
	}
}
