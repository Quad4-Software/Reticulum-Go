// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRaceDiscoveryAnnounceHandler(t *testing.T) {
	var hits atomic.Int32
	h := &interfaceAnnounceHandler{
		requiredValue: 2,
		onDiscovered:  func(*ReceivedAnnounceInfo) { hits.Add(1) },
		validCache:    make(map[string]*ReceivedAnnounceInfo),
		invalidCache:  make(map[string]struct{}),
	}
	apps := make([][]byte, 4)
	for i := range apps {
		app, err := BuildAppData(Info{
			Type:        "AutoInterface",
			TransportID: bytes.Repeat([]byte{byte(0x50 + i)}, 16),
			Name:        "race",
		}, 2, WorkblockExpandRounds)
		if err != nil {
			t.Fatal(err)
		}
		apps[i] = app
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for j := range 40 {
				_ = h.ReceivedAnnounce(nil, nil, apps[j%len(apps)], 0)
				_ = h.ReceivedAnnounce(nil, nil, []byte{0x00}, 0)
			}
		})
	}
	wg.Wait()

	h.mu.Lock()
	validN := len(h.validCache)
	invalidN := len(h.invalidCache)
	busy := h.validating
	h.mu.Unlock()
	if busy {
		t.Fatal("validating stuck true after race")
	}
	if validN > discoveryAnnounceCacheMax || invalidN > discoveryAnnounceCacheMax {
		t.Fatalf("cache overflow valid=%d invalid=%d", validN, invalidN)
	}
	if hits.Load() == 0 && validN == 0 {
		t.Fatal("expected some valid discoveries under race")
	}
}
