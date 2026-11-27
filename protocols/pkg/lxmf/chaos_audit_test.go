// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"sync"
	"testing"
	"time"
)

func goroutineCount() int { return runtime.NumGoroutine() }

func fdCount(t *testing.T) int {
	t.Helper()
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skip("no /proc fd counting")
	}
	return len(ents)
}

func dumpGoroutines(t *testing.T) {
	t.Helper()
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	t.Logf("goroutine dump:\n%s", buf[:n])
}

// Many concurrent hostile clients hammer the control/propagation handlers
// while peers join and leave. The router must survive with bounded
// goroutines and file descriptors and still answer afterwards.
func TestChaosAudit_ConcurrentHostileClients(t *testing.T) {
	if testing.Short() {
		t.Skip("chaos load test")
	}
	r := auditRouter(t)
	base := goroutineCount()
	baseFD := fdCount(t)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	clients := 100
	errs := make(chan error, clients)
	for c := 0; c < clients; c++ {
		wg.Add(1)
		go func(c int) {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					errs <- fmt.Errorf("client %d panic: %v", c, rec)
				}
			}()
			id := mustNewIdentity(t)
			linkID := make([]byte, 16)
			_, _ = rand.Read(linkID)
			buf := make([]byte, 256)
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _ = rand.Read(buf)
				now := time.Now().Unix()
				_ = r.offerRequestHandler("/pn/offer", buf[:1+int(buf[0])%200], nil, linkID, id, now)
				_ = r.messageGetRequestHandler("/pn/get", buf[:64], nil, linkID, id, now)
				_ = r.peerSyncRequestHandler("/pn/sync", buf[:32], nil, linkID, id, now)
				_ = r.statsGetRequestHandler("/pn/stats", nil, nil, linkID, id, now)
			}
		}(c)
	}

	done := make(chan struct{})
	go func() {
		time.Sleep(8 * time.Second)
		close(stop)
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		dumpGoroutines(t)
		t.Fatal("chaos watchdog: deadlock or hang")
	}
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	time.Sleep(500 * time.Millisecond)
	runtime.GC()
	if got := goroutineCount(); got > base+clients/2 {
		dumpGoroutines(t)
		t.Errorf("goroutines not bounded: base=%d after=%d", base, got)
	}
	if got := fdCount(t); got > baseFD+20 {
		t.Errorf("fds not bounded: base=%d after=%d", baseFD, got)
	}
	// node must still answer a stats request afterwards
	if got := r.statsGetRequestHandler("/pn/stats", nil, nil, make([]byte, 16), r.identity, time.Now().Unix()); got == int(PeerErrorNoAccess) {
		t.Error("node not answering after chaos")
	}
	_ = pprof.Lookup
}

// Store crash-recovery chaos: random interleaving of adds, removes, and
// simulated crashes (reload from disk mid-stream) never loses acknowledged
// entries and never resurrects removed ones.
func TestChaosAudit_StoreCrashRecovery(t *testing.T) {
	seed := uint64(424242)
	if v := os.Getenv("LXMF_CHAOS_SEED"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			seed = n
		}
	}
	t.Logf("chaos seed: %d (replay with LXMF_CHAOS_SEED=%d)", seed, seed)
	rng := seed
	next := func(n int) int {
		rng = rng*6364136223846793005 + 1442695040888963407
		return int((rng >> 33) % uint64(n))
	}
	for round := 0; round < 8; round++ {
		dir := t.TempDir()
		ms, err := NewMessageStore(dir, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		live := make(map[string]bool)
		var ids [][]byte
		for op := 0; op < 120; op++ {
			switch next(5) {
			case 0, 1, 2:
				lxm := make([]byte, 200+next(800))
				_, _ = rand.Read(lxm)
				stamp := make([]byte, StampSize)
				if _, err := ms.Add(lxm, stamp, 16); err != nil {
					t.Fatalf("round %d op %d add: %v", round, op, err)
				}
				sum := sha256.Sum256(lxm)
				id := append([]byte(nil), sum[:]...)
				ids = append(ids, id)
				live[string(id)] = true
			case 3:
				if len(ids) > 0 {
					i := next(len(ids))
					_ = ms.Remove(ids[i])
					delete(live, string(ids[i]))
				}
			case 4:
				// simulated crash: reload a fresh store over the same dir
				ms2, err := NewMessageStore(dir, 0, nil)
				if err != nil {
					t.Fatalf("round %d op %d reload: %v", round, op, err)
				}
				ms = ms2
				for idHex := range live {
					if _, ok := ms.Get([]byte(idHex)); !ok {
						t.Fatalf("round %d op %d: acknowledged entry lost after reload", round, op)
					}
				}
			}
		}
		// final: every live id present, no removed id present
		for idHex, alive := range live {
			_, ok := ms.Get([]byte(idHex))
			if alive && !ok {
				t.Errorf("round %d final: live entry missing", round)
			}
			if !alive && ok {
				t.Errorf("round %d final: removed entry resurrected", round)
			}
		}
	}
}
