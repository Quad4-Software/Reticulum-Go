// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestHandlerPoolGoroutineBudget(t *testing.T) {
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
