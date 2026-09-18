// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/discovery"
)

func TestGoldenAutoconnectInterfaceName(t *testing.T) {
	cases := []struct {
		info discovery.Info
		want string
	}{
		{
			info: discovery.Info{Type: "BackboneInterface", Name: "peer", ReachableOn: "192.0.2.1", Port: 4242, HasPort: true},
			want: "peer (192.0.2.1:4242)",
		},
		{
			info: discovery.Info{Type: "I2PInterface", Name: "", ReachableOn: "abc.b32.i2p"},
			want: "Discovered I2PInterface (abc.b32.i2p)",
		},
	}
	for i, tc := range cases {
		got := autoconnectInterfaceName(&discovery.ReceivedAnnounceInfo{Info: tc.info})
		if got != tc.want {
			t.Fatalf("case %d: got %q want %q", i, got, tc.want)
		}
	}
}
