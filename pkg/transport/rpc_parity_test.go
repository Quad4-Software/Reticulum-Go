// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"

	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// pythonIfaceStatKeys is the key set RNS get_interface_stats assigns on
// every interface entry. Python tools index them unconditionally, so a
// missing key becomes a KeyError in rnstatus and friends.
var pythonIfaceStatKeys = []string{
	"name", "short_name", "hash", "type", "mtu",
	"rxb", "txb", "rxs", "txs",
	"arxb", "atxb", "arxc", "atxc", "arxs", "atxs",
	"prxb", "ptxb", "prxc", "ptxc", "prxs", "ptxs",
	"txdrp", "txdrb", "txstalled", "txbuffered",
	"incoming_announce_frequency", "outgoing_announce_frequency",
	"incoming_pr_frequency", "outgoing_pr_frequency",
	"announce_rate_target", "announce_rate_penalty", "announce_rate_grace",
	"held_announces",
	"burst_active", "burst_activated", "burst_count",
	"pr_burst_active", "pr_burst_activated", "pr_burst_count",
	"status", "mode", "gravity", "announces_to_internal",
	"clients", "autoconnect_source",
	"ifac_signature", "ifac_size", "ifac_netname",
	"protocol_violations", "ifac_violations", "packet_filter_hits",
}

func TestInterfaceStatEmitsPythonKeySet(t *testing.T) {
	raw, err := msgpack.Marshal(InterfaceStat{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := msgpack.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range pythonIfaceStatKeys {
		if _, ok := out[k]; !ok {
			t.Fatalf("InterfaceStat missing python-required key %q", k)
		}
	}
}
