// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"crypto/sha256"
	"sync/atomic"
	"testing"
)

type discoveryCacheOracle struct {
	ValidHits   int
	InvalidHits int
	Callbacks   int32
}

func TestOracleDiscoveryValidCacheReplay(t *testing.T) {
	var callbacks atomic.Int32
	h := &interfaceAnnounceHandler{
		requiredValue: 2,
		onDiscovered:  func(*ReceivedAnnounceInfo) { callbacks.Add(1) },
		validCache:    make(map[string]*ReceivedAnnounceInfo),
		invalidCache:  make(map[string]struct{}),
	}
	app, err := BuildAppData(Info{
		Type:        "AutoInterface",
		TransportID: bytes.Repeat([]byte{0x31}, 16),
		Name:        "oracle",
	}, 2, WorkblockExpandRounds)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.ReceivedAnnounce(nil, nil, app, 0); err != nil {
		t.Fatal(err)
	}
	if err := h.ReceivedAnnounce(nil, nil, app, 0); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(app[1:])
	oracle := discoveryCacheOracle{
		ValidHits: len(h.validCache),
		Callbacks: callbacks.Load(),
	}
	if oracle.ValidHits != 1 {
		t.Fatalf("valid cache entries=%d want 1", oracle.ValidHits)
	}
	if oracle.Callbacks != 2 {
		t.Fatalf("callbacks=%d want 2 (validate once, cache replay once)", oracle.Callbacks)
	}
	if _, ok := h.validCache[string(sum[:])]; !ok {
		t.Fatal("missing valid cache key")
	}
}

func TestOracleDiscoveryInvalidCacheBlocksReplay(t *testing.T) {
	var callbacks atomic.Int32
	h := &interfaceAnnounceHandler{
		requiredValue: 200,
		onDiscovered:  func(*ReceivedAnnounceInfo) { callbacks.Add(1) },
		validCache:    make(map[string]*ReceivedAnnounceInfo),
		invalidCache:  make(map[string]struct{}),
	}
	app, err := BuildAppData(Info{
		Type:        "AutoInterface",
		TransportID: bytes.Repeat([]byte{0x32}, 16),
		Name:        "bad",
	}, 2, WorkblockExpandRounds)
	if err != nil {
		t.Fatal(err)
	}
	_ = h.ReceivedAnnounce(nil, nil, app, 0)
	h.requiredValue = 0
	_ = h.ReceivedAnnounce(nil, nil, app, 0)
	if callbacks.Load() != 0 {
		t.Fatal("invalid cache must suppress callbacks even if cost drops")
	}
	if len(h.invalidCache) != 1 {
		t.Fatalf("invalid cache size=%d want 1", len(h.invalidCache))
	}
}

func TestAdversarialDiscoveryAnnounceBodies(t *testing.T) {
	h := &interfaceAnnounceHandler{
		requiredValue: 2,
		validCache:    make(map[string]*ReceivedAnnounceInfo),
		invalidCache:  make(map[string]struct{}),
	}
	bodies := [][]byte{
		nil,
		{},
		{0x00},
		{0xff},
		{0x00, 0x01},
		bytes.Repeat([]byte{0xaa}, 8),
		append([]byte{0x00}, bytes.Repeat([]byte{0xff}, 4096)...),
	}
	for i, body := range bodies {
		if err := h.ReceivedAnnounce(nil, nil, body, 0); err != nil {
			t.Fatalf("case %d returned error: %v", i, err)
		}
	}
}

func TestAdversarialDiscoverySingleFlightIgnoresPeers(t *testing.T) {
	var hits atomic.Int32
	h := &interfaceAnnounceHandler{
		requiredValue: 2,
		onDiscovered:  func(*ReceivedAnnounceInfo) { hits.Add(1) },
		validCache:    make(map[string]*ReceivedAnnounceInfo),
		invalidCache:  make(map[string]struct{}),
		validating:    true,
	}
	app, err := BuildAppData(Info{
		Type:        "AutoInterface",
		TransportID: bytes.Repeat([]byte{0x33}, 16),
		Name:        "sf",
	}, 2, WorkblockExpandRounds)
	if err != nil {
		t.Fatal(err)
	}
	_ = h.ReceivedAnnounce(nil, nil, app, 0)
	if hits.Load() != 0 || len(h.validCache) != 0 {
		t.Fatal("single-flight must drop without caching")
	}
}
