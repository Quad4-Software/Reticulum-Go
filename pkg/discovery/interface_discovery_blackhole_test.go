// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"testing"
)

func TestDiscoveryInfoBlackholedTransportAndRemote(t *testing.T) {
	tid := bytes.Repeat([]byte{0x11}, 16)
	rid := bytes.Repeat([]byte{0x22}, 16)
	info := &ReceivedAnnounceInfo{
		Info:           Info{TransportID: tid},
		RemoteIdentity: rid,
	}
	if discoveryInfoBlackholed(info, nil) {
		t.Fatal("nil isBlackholed must not drop")
	}
	if discoveryInfoBlackholed(info, func(h []byte) bool { return false }) {
		t.Fatal("non-matching blackhole must not drop")
	}
	if !discoveryInfoBlackholed(info, func(h []byte) bool { return bytes.Equal(h, tid) }) {
		t.Fatal("blackholed transport id must drop")
	}
	if !discoveryInfoBlackholed(info, func(h []byte) bool { return bytes.Equal(h, rid) }) {
		t.Fatal("blackholed remote identity must drop")
	}
}
