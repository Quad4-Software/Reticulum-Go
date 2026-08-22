// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"encoding/hex"
	"testing"
)

// Golden endpoint hashes match Python RNS Discovery.endpoint_hash
// (Identity.full_hash of reachable_on plus optional :port).
var goldenEndpointHashes = []struct {
	reachableOn string
	port        int64
	hasPort     bool
	wantHex     string
}{
	{"192.0.2.1", 4242, true, "677c7de9d3a42a6a79970a0fa1403660338d886f1ab52b687961d8d13f48d230"},
	{"10.0.0.1", 9000, true, "b382a505bf8f6c28ae5e54883182ae919106a3bfb11bafa61fb5587f2d314059"},
	{"peer.b32.i2p", 0, false, "5eeaada3a6e5e44fe0b589189ada143cc3e9fc2b6c26f8f94473487027b37cc6"},
}

func TestGoldenEndpointHashVectors(t *testing.T) {
	for i, tc := range goldenEndpointHashes {
		info := &ReceivedAnnounceInfo{
			Info: Info{
				ReachableOn: tc.reachableOn,
				Port:        tc.port,
				HasPort:     tc.hasPort,
			},
		}
		got := EndpointHash(info)
		if len(got) != 32 {
			t.Fatalf("case %d: len=%d", i, len(got))
		}
		if hex.EncodeToString(got) != tc.wantHex {
			t.Fatalf("case %d: got %s want %s", i, hex.EncodeToString(got), tc.wantHex)
		}
	}
}

func TestGoldenAutoconnectTypes(t *testing.T) {
	want := []string{"BackboneInterface", "TCPServerInterface", "I2PInterface"}
	for _, typ := range want {
		if _, ok := AutoconnectTypes[typ]; !ok {
			t.Fatalf("missing autoconnect type %q", typ)
		}
	}
	if len(AutoconnectTypes) != len(want) {
		t.Fatalf("AutoconnectTypes=%d want %d", len(AutoconnectTypes), len(want))
	}
}
