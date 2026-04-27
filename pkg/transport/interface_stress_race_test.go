package transport

import (
	"bytes"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
)

func TestStressConcurrentUnregisterReplacePath(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	dest := bytes.Repeat([]byte{0xAA}, 16)
	nextHop := bytes.Repeat([]byte{0xBB}, 16)

	const workers = 24
	const iters = 150
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				iface := mockIface("wan", true)
				_ = tr.ReplaceInterface("wan", iface)
				tr.UpdatePath(dest, nextHop, "wan", 2)
				_ = tr.HasPath(dest)
				_ = tr.NextHopInterface(dest)
				tr.UnregisterInterface("wan")
			}
		}()
	}
	wg.Wait()
}

func TestStressConcurrentAnnounceWhileReplace(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()
	SetTransportInstance(tr)
	defer SetTransportInstance(nil)

	a := mockIface("a", true)
	if err := tr.RegisterInterface("a", a); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface("b", mockIface("b", true)); err != nil {
		t.Fatal(err)
	}

	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Add(2)
	pkt := []byte{0x01, 0x02, 0x03}
	go func() {
		defer wg.Done()
		for !stop.Load() {
			_ = SendAnnounce(pkt)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 120 && !stop.Load(); i++ {
			ni := mockIface("a", true)
			_ = tr.ReplaceInterface("a", ni)
			time.Sleep(time.Millisecond)
		}
		stop.Store(true)
	}()
	wg.Wait()
}

func TestStressConcurrentReadersDuringUnregister(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	iface := mockIface("eth", true)
	if err := tr.RegisterInterface("eth", iface); err != nil {
		t.Fatal(err)
	}
	dest := bytes.Repeat([]byte{0xCC}, 16)
	tr.UpdatePath(dest, bytes.Repeat([]byte{0xDD}, 16), "eth", 1)

	var wg sync.WaitGroup
	const readers = 16
	wg.Add(readers + 1)
	for r := 0; r < readers; r++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_ = tr.HasPath(dest)
				_ = tr.NextHop(dest)
				_ = tr.NextHopInterface(dest)
				_, _ = tr.GetInterface("eth")
			}
		}()
	}
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			tr.UnregisterInterface("eth")
			_ = tr.RegisterInterface("eth", mockIface("eth", true))
			tr.UpdatePath(dest, bytes.Repeat([]byte{0xDD}, 16), "eth", 1)
		}
	}()
	wg.Wait()
}

// TestGoroutineBudgetAfterUnregisterStress is opt-in (set RETICULUM_STRESS_LEAK=1) because
// NumGoroutine is noisy under parallel CI; use with: go test -race ./pkg/transport -run Budget -count=3
func TestGoroutineBudgetAfterUnregisterStress(t *testing.T) {
	if testing.Short() || os.Getenv("RETICULUM_STRESS_LEAK") == "" {
		t.Skip()
	}
	before := runtime.NumGoroutine()
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()
	dest := bytes.Repeat([]byte{0x11}, 16)
	nh := bytes.Repeat([]byte{0x22}, 16)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 120; i++ {
				iface := mockIface("w", true)
				_ = tr.ReplaceInterface("w", iface)
				tr.UpdatePath(dest, nh, "w", 1)
				tr.UnregisterInterface("w")
			}
		}()
	}
	wg.Wait()
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+64 {
		t.Fatalf("goroutine budget: before=%d after=%d (possible leak; run with -race)", before, after)
	}
}

func TestConcurrentTransportCloseAndReplace(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	tr := NewTransport(&common.ReticulumConfig{})
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				_ = tr.ReplaceInterface("x", mockIface("x", true))
			}
		}
	}()
	time.Sleep(40 * time.Millisecond)
	close(done)
	wg.Wait()
	_ = tr.Close()
}
