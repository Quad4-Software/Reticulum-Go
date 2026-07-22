// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"crypto/sha256"
	"sync/atomic"
	"testing"
)

func TestInterfaceDiscoveryInvalidStampCache(t *testing.T) {
	h := &interfaceAnnounceHandler{
		requiredValue: 200,
		validCache:    make(map[string]*ReceivedAnnounceInfo),
		invalidCache:  make(map[string]struct{}),
	}
	app, err := BuildAppData(Info{
		Type:        "AutoInterface",
		TransportID: bytes.Repeat([]byte{0x11}, 16),
		Name:        "node",
	}, 2, WorkblockExpandRounds)
	if err != nil {
		t.Fatalf("BuildAppData: %v", err)
	}
	if err := h.ReceivedAnnounce(nil, nil, app, 0); err != nil {
		t.Fatalf("first announce: %v", err)
	}
	sum := sha256.Sum256(app[1:])
	key := string(sum[:])
	if _, ok := h.invalidCache[key]; !ok {
		t.Fatal("expected invalid stamp cache entry")
	}
	var validated atomic.Int32
	h.requiredValue = 0
	h.onDiscovered = func(*ReceivedAnnounceInfo) { validated.Add(1) }
	if err := h.ReceivedAnnounce(nil, nil, app, 0); err != nil {
		t.Fatalf("cached announce: %v", err)
	}
	if validated.Load() != 0 {
		t.Fatal("invalid cache should short-circuit without callback")
	}
}

func TestInterfaceDiscoveryValidStampCache(t *testing.T) {
	var hits atomic.Int32
	h := &interfaceAnnounceHandler{
		requiredValue: 2,
		onDiscovered:  func(*ReceivedAnnounceInfo) { hits.Add(1) },
		validCache:    make(map[string]*ReceivedAnnounceInfo),
		invalidCache:  make(map[string]struct{}),
	}
	app, err := BuildAppData(Info{
		Type:        "AutoInterface",
		TransportID: bytes.Repeat([]byte{0x11}, 16),
		Name:        "node",
	}, 2, WorkblockExpandRounds)
	if err != nil {
		t.Fatalf("BuildAppData: %v", err)
	}
	if _, err := ValidateAndDecode(app, 2, WorkblockExpandRounds); err != nil {
		t.Fatalf("precheck ValidateAndDecode: %v", err)
	}
	if err := h.ReceivedAnnounce(nil, nil, app, 0); err != nil {
		t.Fatalf("first announce: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d want 1", hits.Load())
	}
	sum := sha256.Sum256(app[1:])
	key := string(sum[:])
	if _, ok := h.validCache[key]; !ok {
		t.Fatal("expected valid stamp cache entry")
	}
	if err := h.ReceivedAnnounce(nil, nil, app, 0); err != nil {
		t.Fatalf("second announce: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits=%d want 2 from cache", hits.Load())
	}
}

func TestInterfaceDiscoverySingleFlightDropsConcurrent(t *testing.T) {
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
		TransportID: bytes.Repeat([]byte{0x22}, 16),
		Name:        "node2",
	}, 2, WorkblockExpandRounds)
	if err != nil {
		t.Fatalf("BuildAppData: %v", err)
	}
	if err := h.ReceivedAnnounce(nil, nil, app, 0); err != nil {
		t.Fatalf("concurrent announce: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("single-flight drop should not invoke callback")
	}
	if len(h.validCache) != 0 || len(h.invalidCache) != 0 {
		t.Fatal("dropped announce must not populate caches")
	}
}
