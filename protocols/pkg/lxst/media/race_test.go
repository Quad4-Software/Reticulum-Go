// SPDX-License-Identifier: LicenseRef-Reticulum
package media_test

import (
	"sync"
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/media"
)

func TestJitterRacePushPop(t *testing.T) {
	jb := media.NewJitterBuffer(40, 64)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 1000 {
			jb.Push(uint16(i), uint32(i), []byte{byte(i)})
		}
	}()
	go func() {
		defer wg.Done()
		now := time.Now()
		for i := range 1000 {
			_, _ = jb.PopReady(now.Add(time.Duration(i) * time.Millisecond))
		}
	}()
	wg.Wait()
}

func TestAdaptiveRace(t *testing.T) {
	ac := media.NewAdaptiveController()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 200 {
			ac.Update(media.LinkMetrics{RTT: 0.1, LossRate: 0.01, JitterMs: 20, Q: 0.8, SNR: 10})
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			_ = ac.Bitrate()
			_ = ac.UseFEC()
			_ = ac.JitterMs()
		}
	}()
	wg.Wait()
}

func TestJitterWraparound(t *testing.T) {
	jb := media.NewJitterBuffer(20, 8)
	now := time.Now()
	jb.Push(65534, 1, []byte{1})
	jb.Push(65535, 2, []byte{2})
	jb.Push(0, 3, []byte{3})
	f, ok := jb.PopReady(now)
	if !ok || f.Sequence != 65534 {
		t.Fatalf("first %v ok=%v", f.Sequence, ok)
	}
	f, ok = jb.PopReady(now.Add(10 * time.Millisecond))
	if !ok || f.Sequence != 65535 {
		t.Fatalf("second %v", f.Sequence)
	}
	f, ok = jb.PopReady(now.Add(20 * time.Millisecond))
	if !ok || f.Sequence != 0 {
		t.Fatalf("wrap %v", f.Sequence)
	}
}

func TestJitterLatePacketDropped(t *testing.T) {
	jb := media.NewJitterBuffer(20, 8)
	now := time.Now()
	jb.Push(1, 1, []byte{1})
	jb.Push(2, 2, []byte{2})
	_, _ = jb.PopReady(now)
	_, _ = jb.PopReady(now.Add(10 * time.Millisecond))
	jb.Push(1, 1, []byte{9})
	if jb.Depth() != 0 {
		t.Fatalf("late packet kept depth=%d", jb.Depth())
	}
}

func TestJitterMaxFramesDropsOldest(t *testing.T) {
	jb := media.NewJitterBuffer(20, 2)
	jb.Push(1, 1, []byte{1})
	jb.Push(2, 2, []byte{2})
	jb.Push(3, 3, []byte{3})
	if jb.Depth() != 2 {
		t.Fatalf("depth %d", jb.Depth())
	}
	now := time.Now()
	f, ok := jb.PopReady(now)
	if !ok || f.Sequence != 2 {
		t.Fatalf("expected seq 2 after dropping oldest, got seq=%d ok=%v", f.Sequence, ok)
	}
}

func FuzzJitterBuffer(f *testing.F) {
	f.Add(uint16(1), uint16(2), uint16(3))
	f.Fuzz(func(t *testing.T, a, b, c uint16) {
		jb := media.NewJitterBuffer(20, 8)
		now := time.Now()
		pushed := map[uint16]struct{}{a: {}, b: {}, c: {}}
		jb.Push(a, 1, []byte{1})
		jb.Push(b, 2, []byte{2})
		jb.Push(c, 3, []byte{3})
		for i := range 8 {
			fr, ok := jb.PopReady(now.Add(time.Duration(i) * 50 * time.Millisecond))
			if ok {
				if _, known := pushed[fr.Sequence]; !known {
					t.Fatalf("fabricated seq %d", fr.Sequence)
				}
			}
		}
	})
}
