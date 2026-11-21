// SPDX-License-Identifier: LicenseRef-Reticulum
package media_test

import (
	"testing"
	"time"

	"github.com/Quad4-Software/pbt/pkg/pbt"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/media"
)

func TestJitterBufferReorderThenPop(t *testing.T) {
	jb := media.NewJitterBuffer(20, 8)
	now := time.Now()
	jb.Push(2, 40, []byte("b"))
	jb.Push(1, 20, []byte("a"))
	f1, ok := jb.PopReady(now.Add(5 * time.Millisecond))
	if !ok || f1.Sequence != 1 || string(f1.Payload) != "a" {
		t.Fatalf("expected seq 1 payload a, got %+v ok=%v", f1, ok)
	}
	f2, ok := jb.PopReady(now.Add(10 * time.Millisecond))
	if !ok || f2.Sequence != 2 || string(f2.Payload) != "b" {
		t.Fatalf("expected seq 2 payload b, got %+v ok=%v", f2, ok)
	}
}

func TestJitterBufferMissingFrameCountsLoss(t *testing.T) {
	jb := media.NewJitterBuffer(40, 8)
	now := time.Now()
	jb.Push(1, 1, []byte{1})
	jb.Push(3, 3, []byte{3})
	f, ok := jb.PopReady(now)
	if !ok || f.Sequence != 1 {
		t.Fatalf("first %+v", f)
	}
	_, ok = jb.PopReady(now.Add(5 * time.Millisecond))
	if ok {
		t.Fatal("seq 2 should still be waiting")
	}
	_, ok = jb.PopReady(now.Add(50 * time.Millisecond))
	if ok {
		t.Fatal("skip should not return a fabricated frame")
	}
	f, ok = jb.PopReady(now.Add(55 * time.Millisecond))
	if !ok || f.Sequence != 3 {
		t.Fatalf("expected seq 3 after skip, got %+v ok=%v", f, ok)
	}
	if jb.LossRate() == 0 {
		t.Fatal("skip must count loss")
	}
}

func TestJitterBufferInOrderPop(t *testing.T) {
	gen := pbt.IntRange(1, 32)
	pbt.Check(t, pbt.ForAll("in-order jitter pops sequential seqs", gen, func(n int) bool {
		jb := media.NewJitterBuffer(20, n+4)
		now := time.Now()
		for i := range n {
			jb.Push(uint16(i), uint32(i), []byte{byte(i)})
		}
		for i := range n {
			f, ok := jb.PopReady(now.Add(time.Duration(i) * time.Millisecond))
			if !ok || f.Sequence != uint16(i) {
				return false
			}
		}
		return true
	}), pbt.WithRuns(50))
}
