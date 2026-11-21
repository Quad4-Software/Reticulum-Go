// SPDX-License-Identifier: LicenseRef-Reticulum
package media_test

import (
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/media"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestJitterBufferInOrder(t *testing.T) {
	jb := media.NewJitterBuffer(20, 8)
	now := time.Now()
	jb.Push(1, 20, []byte("a"))
	jb.Push(2, 40, []byte("b"))
	f1, ok := jb.PopReady(now.Add(30 * time.Millisecond))
	if !ok || f1.Sequence != 1 {
		t.Fatalf("expected frame 1")
	}
	f2, ok := jb.PopReady(now.Add(60 * time.Millisecond))
	if !ok || f2.Sequence != 2 {
		t.Fatalf("expected frame 2")
	}
}

func TestJitterBufferReorder(t *testing.T) {
	jb := media.NewJitterBuffer(20, 8)
	now := time.Now()
	jb.Push(2, 40, []byte("b"))
	jb.Push(1, 20, []byte("a"))
	f1, ok := jb.PopReady(now.Add(30 * time.Millisecond))
	if !ok || f1.Sequence != 1 {
		t.Fatalf("expected frame 1 after reorder")
	}
}

func TestAdaptiveControllerSlowLink(t *testing.T) {
	ac := media.NewAdaptiveController()
	start := ac.Bitrate()
	ac.Update(media.LinkMetrics{
		RTT:      3.0,
		LossRate: 0.35,
		JitterMs: 250,
		Q:        0.0,
		SNR:      0,
	})
	if ac.Bitrate() >= start {
		t.Fatalf("expected bitrate to decrease on slow link")
	}
	if !ac.UseFEC() {
		t.Fatalf("expected FEC on lossy link")
	}
	if ac.JitterMs() <= 80 {
		t.Fatalf("expected jitter buffer growth")
	}
}

func TestAdaptiveControllerFastLink(t *testing.T) {
	ac := media.NewAdaptiveController()
	ac.Update(media.LinkMetrics{RTT: 0.05, LossRate: 0, JitterMs: 10, Q: 0.9, SNR: 20})
	if ac.Bitrate() <= proto.MinBitrate {
		t.Fatalf("expected bitrate increase on fast link")
	}
}

func TestSimLossyPath(t *testing.T) {
	jb := media.NewJitterBuffer(60, 32)
	ac := media.NewAdaptiveController()
	now := time.Now()
	jb.Push(0, 0, []byte{0})
	_, _ = jb.PopReady(now)
	for seq := uint16(1); seq < 100; seq++ {
		if seq%5 == 0 {
			continue
		}
		jb.Push(seq, uint32(seq*20), []byte{byte(seq)})
		tnow := now.Add(time.Duration(seq) * 40 * time.Millisecond)
		_, _ = jb.PopReady(tnow)
		ac.Update(media.LinkMetrics{
			RTT:      0.8,
			LossRate: jb.LossRate(),
			JitterMs: float64(jb.TargetMs()),
		})
		jb.SetTargetMs(ac.JitterMs())
	}
	if jb.LossRate() == 0 {
		t.Fatalf("expected measured loss from gaps")
	}
}
