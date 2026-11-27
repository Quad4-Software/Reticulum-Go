// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"crypto/rand"
	"fmt"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/testconfig"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Store startup load time, per-insert latency, and eviction cost at scale.
// Run with -benchtime=1x for load-time measurements.
func BenchmarkStoreScale(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("load-%d", n), func(b *testing.B) {
			dir := b.TempDir()
			// populate once
			ms, err := NewMessageStore(dir, 0, nil)
			if err != nil {
				b.Fatal(err)
			}
			for i := 0; i < n; i++ {
				lxm := make([]byte, 500)
				_, _ = rand.Read(lxm)
				stamp := make([]byte, StampSize)
				if _, err := ms.Add(lxm, stamp, 16); err != nil {
					b.Fatal(err)
				}
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ms2, err := NewMessageStore(dir, 0, nil)
				if err != nil {
					b.Fatal(err)
				}
				_ = ms2
			}
		})
	}
}

func BenchmarkStoreInsert(b *testing.B) {
	dir := b.TempDir()
	ms, err := NewMessageStore(dir, 0, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lxm := make([]byte, 500)
		_, _ = rand.Read(lxm)
		stamp := make([]byte, StampSize)
		if _, err := ms.Add(lxm, stamp, 16); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStoreEviction(b *testing.B) {
	dir := b.TempDir()
	ms, err := NewMessageStore(dir, 0.05, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lxm := make([]byte, 5000)
		_, _ = rand.Read(lxm)
		stamp := make([]byte, StampSize)
		if _, err := ms.Add(lxm, stamp, 16); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidatePeeringKey(b *testing.B) {
	peeringID := make([]byte, 64)
	key := make([]byte, StampSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ValidatePeeringKey(peeringID, key, 16)
	}
}

func BenchmarkCompileStats200Peers(b *testing.B) {
	r := benchRouter(b)
	for i := 0; i < 200; i++ {
		h := make([]byte, DestinationLength)
		h[0] = byte(i)
		h[1] = byte(i >> 8)
		peer := &Peer{
			DestinationHash: h,
			State:           PeerStateIdle,
			Alive:           true,
		}
		r.peers[peerKey(h)] = peer
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.compileStats()
	}
}

func benchRouter(b testing.TB) *Router {
	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = tr.Close() })
	home := b.TempDir()
	id := mustNewIdentity(b)
	cfg := DefaultConfig()
	cfg.Propagation.EnableNode = true
	r, err := NewRouter(id, tr, RouterOptions{
		Config:      cfg,
		StoragePath: filepath.Join(home, "storage"),
		MessagesDir: filepath.Join(home, "storage", "messages"),
	})
	if err != nil {
		b.Fatal(err)
	}
	if err := r.EnablePropagation(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(r.Stop)
	return r
}

// Store with 200k files: startup load measurement, one shot.
func TestPerfStoreLoad200k(t *testing.T) {
	n := 200000
	if testing.Short() {
		n = 20000
	}
	dir := t.TempDir()
	ms, err := NewMessageStore(dir, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	populate := time.Now()
	lxm := make([]byte, 300)
	_, _ = rand.Read(lxm)
	stamp := make([]byte, StampSize)
	for i := 0; i < n; i++ {
		lxm[0] = byte(i)
		lxm[1] = byte(i >> 8)
		lxm[2] = byte(i >> 16)
		lxm[3] = byte(i >> 24)
		if _, err := ms.Add(lxm, stamp, 16); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("populate %d entries: %v (%.0f inserts/s)", n, time.Since(populate), float64(n)/time.Since(populate).Seconds())

	start := time.Now()
	ms2, err := NewMessageStore(dir, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	loadTime := time.Since(start)
	t.Logf("load %d entries: %v (%.0f entries/s)", n, loadTime, float64(n)/loadTime.Seconds())
	if ms2.Count() != n {
		t.Errorf("count after load: got %d want %d", ms2.Count(), n)
	}
	entries, _ := os.ReadDir(dir)
	t.Logf("dir entries: %d", len(entries))
}
