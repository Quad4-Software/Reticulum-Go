// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"quad4/pbt/pkg/pbt"
)

func TestPBTDiscoveryInvalidCacheShortCircuits(t *testing.T) {
	names := pbt.IntRange(0, 25)
	prop := pbt.ForAll(
		"invalid cache entry always suppresses discovery callback",
		names,
		func(n int) bool {
			name := string(rune('a' + (n % 26)))
			h := &interfaceAnnounceHandler{
				requiredValue: 200,
				validCache:    make(map[string]*ReceivedAnnounceInfo),
				invalidCache:  make(map[string]struct{}),
			}
			app, err := BuildAppData(Info{
				Type:        "AutoInterface",
				TransportID: bytes.Repeat([]byte{0x41}, 16),
				Name:        name,
			}, 2, WorkblockExpandRounds)
			if err != nil {
				return false
			}
			_ = h.ReceivedAnnounce(nil, nil, app, 0)
			sum := sha256.Sum256(app[1:])
			if _, ok := h.invalidCache[string(sum[:])]; !ok {
				return false
			}
			called := false
			h.requiredValue = 0
			h.onDiscovered = func(*ReceivedAnnounceInfo) { called = true }
			_ = h.ReceivedAnnounce(nil, nil, app, 0)
			return !called
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(20), pbt.WithSeed(143))
}

func TestPBTDiscoveryValidCacheStableKey(t *testing.T) {
	prop := pbt.ForAll(
		"repeated valid announce yields one cache entry and N callbacks",
		pbt.IntRange(2, 5),
		func(n int) bool {
			calls := 0
			h := &interfaceAnnounceHandler{
				requiredValue: 2,
				onDiscovered:  func(*ReceivedAnnounceInfo) { calls++ },
				validCache:    make(map[string]*ReceivedAnnounceInfo),
				invalidCache:  make(map[string]struct{}),
			}
			app, err := BuildAppData(Info{
				Type:        "AutoInterface",
				TransportID: bytes.Repeat([]byte{byte(n)}, 16),
				Name:        "pbt",
			}, 2, WorkblockExpandRounds)
			if err != nil {
				return false
			}
			for range n {
				_ = h.ReceivedAnnounce(nil, nil, app, 0)
			}
			return len(h.validCache) == 1 && calls == n
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(12), pbt.WithSeed(144))
}
