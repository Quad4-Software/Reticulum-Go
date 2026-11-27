// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"bytes"
	"encoding/hex"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxmf"
)

func TestAsMapNormalizesPythonPeerKeys(t *testing.T) {
	hash := bytes.Repeat([]byte{0x5a}, lxmf.DestinationLength)
	hexKey := "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a"
	for name, input := range map[string]any{
		"string map raw string key": map[string]any{string(hash): map[string]any{"alive": true}},
		"any map raw string key":    map[any]any{string(hash): map[string]any{"alive": true}},
		"any map array key":         map[any]any{[16]byte(hash): map[string]any{"alive": true}},
	} {
		got := asMap(input)
		if _, ok := got[hexKey]; !ok {
			t.Fatalf("%s did not normalize to hex key: %#v", name, got)
		}
	}
}

func TestOperatorSummaryUsesHexKeyedPeerStats(t *testing.T) {
	hash := bytes.Repeat([]byte{0x44}, lxmf.DestinationLength)
	summary := OperatorSummary{}
	summary.fillFromPropagationStats(map[string]any{
		"total_peers":  int64(1),
		"static_peers": int8(1),
		"uptime":       float32(12),
		"messagestore": map[string]any{"count": uint16(2), "bytes": uint64(3000), "limit": int64(10000)},
		"peers": map[string]any{
			hex.EncodeToString(hash): map[string]any{"alive": true, "state": uint8(1)},
		},
	})
	if summary.TotalPeers != 1 || summary.StaticPeers != 1 || summary.AlivePeers != 1 || summary.SyncingPeers != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.StoreCount != 2 || summary.StoreBytes != 3000 || summary.StoreLimit != 10000 {
		t.Fatalf("unexpected store summary: %+v", summary)
	}
}
