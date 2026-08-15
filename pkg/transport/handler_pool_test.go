// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/protect"
)

func silenceHandlerPoolLogs(t *testing.T) {
	t.Helper()
	prev := debug.GetDebugLevel()
	debug.SetDebugLevel(debug.DebugCritical)
	t.Cleanup(func() { debug.SetDebugLevel(prev) })
}

func TestHandlerPoolStartsSmall(t *testing.T) {
	silenceHandlerPoolLogs(t)
	tr := NewTransport(&common.ReticulumConfig{
		EnableTransport:   true,
		MaxPacketHandlers: common.DefaultMaxPacketHandlers,
	})
	t.Cleanup(func() { _ = tr.Close() })

	iface := common.NewBaseInterface("idle0", common.IFTypeUDP, true)
	tr.HandlePacket([]byte{0x00, 0x00, 0x01}, &iface)
	time.Sleep(20 * time.Millisecond)

	live := int(tr.handlerLive.Load())
	boot := startupHandlerCount(common.DefaultMaxPacketHandlers)
	if live < 1 {
		t.Fatal("no packet handlers started")
	}
	if live > boot+8 {
		t.Fatalf("idle handlers=%d want around %d not %d", live, boot, common.DefaultMaxPacketHandlers)
	}
}

func TestStartupHandlerCountCapsAtMax(t *testing.T) {
	if got := startupHandlerCount(2); got != 2 {
		t.Fatalf("startupHandlerCount(2)=%d", got)
	}
	if got := startupHandlerCount(common.DefaultMaxPacketHandlers); got > common.DefaultMaxPacketHandlers {
		t.Fatalf("startup=%d exceeds max", got)
	}
	if runtime.GOMAXPROCS(0) < common.DefaultMaxPacketHandlers {
		if got := startupHandlerCount(common.DefaultMaxPacketHandlers); got >= common.DefaultMaxPacketHandlers {
			t.Fatalf("startup=%d should be well below default max", got)
		}
	}
}

func TestHandlerPoolDoesNotRampOnLightLoad(t *testing.T) {
	silenceHandlerPoolLogs(t)
	tr := NewTransport(&common.ReticulumConfig{
		EnableTransport:   true,
		MaxPacketHandlers: common.DefaultMaxPacketHandlers,
	})
	t.Cleanup(func() { _ = tr.Close() })

	iface := common.NewBaseInterface("light0", common.IFTypeUDP, true)
	pkt := []byte{0x00, 0x00, 0x01}
	for range 256 {
		tr.HandlePacket(pkt, &iface)
	}
	time.Sleep(20 * time.Millisecond)

	live := int(tr.handlerLive.Load())
	boot := startupHandlerCount(common.DefaultMaxPacketHandlers)
	if live > boot+8 {
		t.Fatalf("light-load handlers=%d boot=%d (must not ramp toward %d)", live, boot, common.DefaultMaxPacketHandlers)
	}
}

func TestHandlerPoolGoroutineBudget(t *testing.T) {
	silenceHandlerPoolLogs(t)
	const n = 8
	tr := NewTransport(&common.ReticulumConfig{
		EnableTransport:   true,
		MaxPacketHandlers: n,
	})
	t.Cleanup(func() { _ = tr.Close() })

	runtime.GC()
	base := runtime.NumGoroutine()
	iface := common.NewBaseInterface("pool0", common.IFTypeUDP, true)
	pkt := []byte{0x00, 0x00, 0x01}
	for range 4000 {
		tr.HandlePacket(pkt, &iface)
	}
	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	delta := runtime.NumGoroutine() - base
	if delta > n*4+32 {
		t.Fatalf("goroutine delta=%d exceeds pool bound n=%d", delta, n)
	}
}

func TestHandlerPoolCloseDoesNotDeadlock(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{
		EnableTransport:   true,
		MaxPacketHandlers: 4,
	})
	hold := make(chan struct{})
	tr.occupyHandlerPoolForTest(hold)
	done := make(chan struct{})
	go func() {
		_ = tr.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		close(hold)
		t.Fatal("Close deadlocked")
	}
	close(hold)
}

func TestHandlerPoolEnqueueCloseRace(t *testing.T) {
	silenceHandlerPoolLogs(t)
	tr := NewTransport(&common.ReticulumConfig{
		EnableTransport:   true,
		MaxPacketHandlers: 4,
	})
	iface := common.NewBaseInterface("race0", common.IFTypeUDP, true)
	pkt := []byte{0x00, 0x00, 0x01}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 8 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					tr.HandlePacket(pkt, &iface)
				}
			}
		})
	}
	time.Sleep(20 * time.Millisecond)
	_ = tr.Close()
	close(stop)
	wg.Wait()
}

func TestHandlerPoolOverflowAlwaysSheds(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	protect.SetDefault(protect.New(protect.Options{Mode: protect.ModeOff}))

	tr := NewTransport(&common.ReticulumConfig{
		EnableTransport:   true,
		MaxPacketHandlers: 2,
	})
	t.Cleanup(func() { _ = tr.Close() })
	hold := make(chan struct{})
	if n := tr.occupyHandlerPoolForTest(hold); n == 0 {
		t.Fatal("handler pool not occupied")
	}
	t.Cleanup(func() { close(hold) })

	iface := common.NewBaseInterface("shed0", common.IFTypeUDP, true)
	before := health.Default.SnapshotIface("shed0").DoSHandler.Total
	tr.HandlePacket([]byte{0x00, 0x00, 0x01}, &iface)
	after := health.Default.SnapshotIface("shed0").DoSHandler.Total
	if after <= before {
		t.Fatalf("overflow did not shed: dos_handler %d -> %d", before, after)
	}
}
