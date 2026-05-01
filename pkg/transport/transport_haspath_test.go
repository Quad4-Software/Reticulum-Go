// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"crypto/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
)

func newHasPathTransport(t testing.TB) (*Transport, *mockInterface) {
	t.Helper()
	tr := NewTransport(&common.ReticulumConfig{})
	t.Cleanup(func() { _ = tr.Close() })

	iface := &mockInterface{}
	iface.Name = "hp_iface"
	iface.Enabled = true
	if err := tr.RegisterInterface(iface.Name, iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}
	return tr, iface
}

func backdatePath(t *Transport, destHash []byte, age time.Duration) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	old, ok := t.paths[string(destHash)]
	if !ok {
		return
	}
	clone := *old
	clone.LastUpdated = time.Now().Add(-age)
	t.paths[string(destHash)] = &clone
}

func randomHash(t testing.TB, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return b
}

func TestHasPath_MissingReturnsFalse(t *testing.T) {
	tr, _ := newHasPathTransport(t)
	if tr.HasPath(randomHash(t, 16)) {
		t.Fatal("HasPath returned true for unknown destination")
	}
}

func TestHasPath_FreshPathReturnsTrue(t *testing.T) {
	tr, iface := newHasPathTransport(t)
	dest := randomHash(t, 16)
	tr.UpdatePath(dest, []byte("next"), iface.Name, 1)

	if !tr.HasPath(dest) {
		t.Fatal("HasPath returned false for fresh path")
	}
	if !tr.HasPath(dest) {
		t.Fatal("HasPath flipped to false on second call")
	}
}

func TestHasPath_BoundaryWithinTTL(t *testing.T) {
	tr, iface := newHasPathTransport(t)
	dest := randomHash(t, 16)
	tr.UpdatePath(dest, []byte("next"), iface.Name, 1)

	backdatePath(tr, dest, time.Duration(PathRequestTTL)*time.Second-time.Second)
	if !tr.HasPath(dest) {
		t.Fatal("path within TTL should still exist")
	}

	tr.mutex.RLock()
	_, present := tr.paths[string(dest)]
	tr.mutex.RUnlock()
	if !present {
		t.Fatal("non-expired path must not be evicted")
	}
}

func TestHasPath_ExpiredEvicted(t *testing.T) {
	tr, iface := newHasPathTransport(t)
	dest := randomHash(t, 16)
	tr.UpdatePath(dest, []byte("next"), iface.Name, 1)

	backdatePath(tr, dest, time.Duration(PathRequestTTL+10)*time.Second)
	if tr.HasPath(dest) {
		t.Fatal("expired path returned true")
	}

	tr.mutex.RLock()
	_, present := tr.paths[string(dest)]
	tr.mutex.RUnlock()
	if present {
		t.Fatal("expired path was not evicted")
	}
}

func TestHasPath_RefreshDuringEscalationKeepsEntry(t *testing.T) {
	tr, iface := newHasPathTransport(t)
	dest := randomHash(t, 16)

	tr.UpdatePath(dest, []byte("next"), iface.Name, 1)
	backdatePath(tr, dest, time.Duration(PathRequestTTL+5)*time.Second)

	var wg sync.WaitGroup
	wg.Go(func() {
		time.Sleep(2 * time.Millisecond)
		tr.UpdatePath(dest, []byte("next2"), iface.Name, 2)
	})

	for range 1000 {
		_ = tr.HasPath(dest)
	}
	wg.Wait()

	tr.mutex.RLock()
	cur, present := tr.paths[string(dest)]
	tr.mutex.RUnlock()
	if !present {
		t.Fatal("refreshed path must be retained when racing with HasPath escalation")
	}
	if time.Since(cur.LastUpdated) > time.Duration(PathRequestTTL)*time.Second {
		t.Fatalf("refreshed path looks expired: age=%v", time.Since(cur.LastUpdated))
	}
}

func TestHasPath_ConcurrentReadersOnSingleExpiredPath(t *testing.T) {
	tr, iface := newHasPathTransport(t)
	dest := randomHash(t, 16)
	tr.UpdatePath(dest, []byte("next"), iface.Name, 1)
	backdatePath(tr, dest, time.Duration(PathRequestTTL+5)*time.Second)

	const readers = 64
	const iters = 2000

	var wg sync.WaitGroup
	wg.Add(readers)
	start := make(chan struct{})
	for range readers {
		go func() {
			defer wg.Done()
			<-start
			for range iters {
				_ = tr.HasPath(dest)
			}
		}()
	}
	close(start)
	wg.Wait()

	tr.mutex.RLock()
	_, present := tr.paths[string(dest)]
	tr.mutex.RUnlock()
	if present {
		t.Fatal("expired path must be evicted exactly once across many readers")
	}
}

func TestHasPath_ConcurrentReadersWritersManyDestinations(t *testing.T) {
	tr, iface := newHasPathTransport(t)

	const dests = 64
	hashes := make([][]byte, dests)
	for i := range hashes {
		hashes[i] = randomHash(t, 16)
		tr.UpdatePath(hashes[i], []byte("nh"), iface.Name, uint8(i%8))
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	for w := range 8 {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			i := seed
			for {
				select {
				case <-stop:
					return
				default:
				}
				h := hashes[i%dests]
				tr.UpdatePath(h, []byte("nh"), iface.Name, uint8(i%8))
				if i%5 == 0 {
					backdatePath(tr, h, time.Duration(PathRequestTTL+1)*time.Second)
				}
				i++
			}
		}(w * 17)
	}

	for r := range 32 {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			i := seed
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = tr.HasPath(hashes[i%dests])
				_ = tr.HopsTo(hashes[i%dests])
				_ = tr.NextHop(hashes[i%dests])
				i++
			}
		}(r * 31)
	}

	time.Sleep(250 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestHasPath_RaceAgainstAllPathReaders(t *testing.T) {
	tr, iface := newHasPathTransport(t)

	const dests = 32
	hashes := make([][]byte, dests)
	for i := range hashes {
		hashes[i] = randomHash(t, 16)
		tr.UpdatePath(hashes[i], []byte("nh"), iface.Name, 1)
		if i%2 == 0 {
			backdatePath(tr, hashes[i], time.Duration(PathRequestTTL+5)*time.Second)
		}
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	for r := range 16 {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			i := seed
			for {
				select {
				case <-stop:
					return
				default:
				}
				h := hashes[i%dests]
				_ = tr.HasPath(h)
				_ = tr.HopsTo(h)
				_ = tr.NextHop(h)
				_ = tr.NextHopInterface(h)
				_ = tr.PathIsUnresponsive(h)
				i++
			}
		}(r * 13)
	}

	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				h := hashes[time.Now().UnixNano()%int64(dests)]
				tr.UpdatePath(h, []byte("nh"), iface.Name, 2)
			}
		})
	}

	time.Sleep(250 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestHasPath_NoEarlyExitOnSlowReader(t *testing.T) {
	tr, iface := newHasPathTransport(t)
	dest := randomHash(t, 16)
	tr.UpdatePath(dest, []byte("next"), iface.Name, 1)
	backdatePath(tr, dest, time.Duration(PathRequestTTL+1)*time.Second)

	var calls int64
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 256 {
				if tr.HasPath(dest) {
					t.Errorf("expired path returned true")
					return
				}
				atomic.AddInt64(&calls, 1)
			}
		})
	}
	wg.Wait()

	if atomic.LoadInt64(&calls) != 8*256 {
		t.Fatalf("expected %d calls, got %d", 8*256, calls)
	}
	tr.mutex.RLock()
	_, present := tr.paths[string(dest)]
	tr.mutex.RUnlock()
	if present {
		t.Fatal("expired path must be evicted")
	}
}

func TestHasPath_DistinctKeysIndependent(t *testing.T) {
	tr, iface := newHasPathTransport(t)
	live := randomHash(t, 16)
	dead := randomHash(t, 16)

	tr.UpdatePath(live, []byte("nh"), iface.Name, 1)
	tr.UpdatePath(dead, []byte("nh"), iface.Name, 1)
	backdatePath(tr, dead, time.Duration(PathRequestTTL+5)*time.Second)

	if !tr.HasPath(live) {
		t.Fatal("live path missing")
	}
	if tr.HasPath(dead) {
		t.Fatal("dead path returned true")
	}
	if !tr.HasPath(live) {
		t.Fatal("live path evicted as side effect of dead lookup")
	}

	tr.mutex.RLock()
	_, liveOK := tr.paths[string(live)]
	_, deadOK := tr.paths[string(dead)]
	tr.mutex.RUnlock()
	if !liveOK {
		t.Fatal("live entry evicted")
	}
	if deadOK {
		t.Fatal("dead entry retained")
	}
}

func BenchmarkHasPath_Hit(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	tr, iface := newHasPathTransport(b)
	dest := randomHash(b, 16)
	tr.UpdatePath(dest, []byte("nh"), iface.Name, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tr.HasPath(dest)
	}
}

func BenchmarkHasPath_Miss(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	tr, _ := newHasPathTransport(b)
	dest := randomHash(b, 16)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tr.HasPath(dest)
	}
}

func BenchmarkHasPath_ParallelMixed(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	tr, iface := newHasPathTransport(b)
	const n = 64
	hashes := make([][]byte, n)
	for i := range hashes {
		hashes[i] = randomHash(b, 16)
		tr.UpdatePath(hashes[i], []byte("nh"), iface.Name, 1)
		if i%4 == 0 {
			backdatePath(tr, hashes[i], time.Duration(PathRequestTTL+1)*time.Second)
		}
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = tr.HasPath(hashes[i%n])
			i++
		}
	})
}
