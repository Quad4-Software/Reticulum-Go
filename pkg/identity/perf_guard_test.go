// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"crypto/rand"
	"testing"
)

// BenchmarkIdentityHashCached guards the cached truncated-hash path used by
// transport relay ID checks. A regression that recomputes SHA-256 every call
// shows up as both higher ns/op and allocs/op.
func BenchmarkIdentityHashCached(b *testing.B) {
	id, err := New()
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	_ = id.Hash()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = id.Hash()
	}
}

// BenchmarkRememberUnchanged measures the hot re-announce path where the
// destination is already known with identical packet and app data.
func BenchmarkRememberUnchanged(b *testing.B) {
	knownDestinationsLock.Lock()
	knownDestinations = make(map[string][]any)
	knownDestinationsLock.Unlock()
	b.Cleanup(func() {
		knownDestinationsLock.Lock()
		knownDestinations = make(map[string][]any)
		knownDestinationsLock.Unlock()
	})

	pub := make([]byte, 64)
	if _, err := rand.Read(pub); err != nil {
		b.Fatalf("rand: %v", err)
	}
	dest := make([]byte, 16)
	if _, err := rand.Read(dest); err != nil {
		b.Fatalf("rand: %v", err)
	}
	pkt := []byte("announce-packet-body")
	app := []byte("app-data")
	Remember(pkt, dest, pub, app)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Remember(pkt, dest, pub, app)
	}
}

// TestRememberUnchangedAllocBudget fails if the unchanged Remember fast path
// starts allocating again (map key hex encode is the only expected cost).
func TestRememberUnchangedAllocBudget(t *testing.T) {
	knownDestinationsLock.Lock()
	knownDestinations = make(map[string][]any)
	knownDestinationsLock.Unlock()
	t.Cleanup(func() {
		knownDestinationsLock.Lock()
		knownDestinations = make(map[string][]any)
		knownDestinationsLock.Unlock()
	})

	pub := make([]byte, 64)
	dest := make([]byte, 16)
	pkt := []byte("pkt")
	app := []byte("app")
	_, _ = rand.Read(pub)
	_, _ = rand.Read(dest)
	Remember(pkt, dest, pub, app)

	allocs := testing.AllocsPerRun(1000, func() {
		Remember(pkt, dest, pub, app)
	})
	// hex map key string is one alloc. Allow a little headroom for GC noise.
	if allocs > 2 {
		t.Fatalf("Remember unchanged allocs=%.1f want <= 2", allocs)
	}
}

// TestIdentityHashCachedAllocBudget ensures Hash returns a copy of the cached
// truncated hash without rehashing public key material.
func TestIdentityHashCachedAllocBudget(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = id.Hash()
	allocs := testing.AllocsPerRun(1000, func() {
		_ = id.Hash()
	})
	if allocs > 1 {
		t.Fatalf("Identity.Hash cached allocs=%.1f want <= 1", allocs)
	}
}
