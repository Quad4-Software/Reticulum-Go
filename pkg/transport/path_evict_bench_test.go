package transport

import (
	"bytes"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
)

func BenchmarkCleanupExpiredPaths_350k(b *testing.B) {
	cfg := unstampedTestConfig(true)
	cfg.MaxInMemoryPaths = 600000
	cfg.MaxInMemoryPathsSet = true
	tr := NewTransport(cfg)
	defer func() { _ = tr.Close() }()
	iface := newSimIface("s")
	_ = tr.RegisterInterface("s", iface)

	tr.mutex.Lock()
	now := time.Now()
	for i := range 350000 {
		var k [PathMapKeySize]byte
		k[0] = byte(i)
		k[1] = byte(i >> 8)
		k[2] = byte(i >> 16)
		tr.paths[k] = &common.Path{
			NextHop:     bytes.Repeat([]byte{2}, 16),
			Interface:   iface,
			HopCount:    1,
			Hops:        1,
			LastUpdated: now,
			Expires:     now.Add(24 * time.Hour),
		}
		tr.pathStates[k] = StateUnknown
		tr.seenAnnounces[k] = now
	}
	tr.mutex.Unlock()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tr.cleanupExpiredPaths()
		tr.cleanupSeenAnnounces()
	}
}

func BenchmarkUpdatePath_AtCap350k(b *testing.B) {
	cfg := unstampedTestConfig(true)
	cfg.MaxInMemoryPaths = 350000
	cfg.MaxInMemoryPathsSet = true
	tr := NewTransport(cfg)
	defer func() { _ = tr.Close() }()
	iface := newSimIface("s")
	_ = tr.RegisterInterface("s", iface)

	var k [PathMapKeySize]byte
	for i := range 350000 {
		k[0] = byte(i)
		k[1] = byte(i >> 8)
		k[2] = byte(i >> 16)
		tr.UpdatePath(k[:16], bytes.Repeat([]byte{2}, 16), "s", 1)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		j := 350000 + i
		k[0] = byte(j)
		k[1] = byte(j >> 8)
		k[2] = byte(j >> 16)
		tr.UpdatePath(k[:16], bytes.Repeat([]byte{3}, 16), "s", 2)
	}
}
