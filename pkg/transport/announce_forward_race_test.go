// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
)

func useFastPathfinder(t *testing.T) {
	t.Helper()
	zero := 0.0
	simHooksMu.Lock()
	simPathfinderRW = &zero
	simHooksMu.Unlock()
	t.Cleanup(func() {
		simHooksMu.Lock()
		simPathfinderRW = nil
		simHooksMu.Unlock()
	})
}

func TestScheduleAnnounceForwardJob_BacklogFullDrops(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	tr.pendingAnnounceMu.Lock()
	for range MaxPendingAnnounceForwards {
		tr.pendingAnnounceJobs = append(tr.pendingAnnounceJobs, delayedAnnounceJob{
			due:  time.Now().Add(time.Hour),
			data: []byte{0x01, 0x00},
		})
	}
	tr.pendingAnnounceMu.Unlock()

	tr.scheduleAnnounceForward([]byte{0x01, 0x00}, hash16{}, []byte("dest"), nil)
	tr.pendingAnnounceMu.Lock()
	n0 := len(tr.pendingAnnounceJobs)
	tr.pendingAnnounceMu.Unlock()
	if n0 != MaxPendingAnnounceForwards {
		t.Fatalf("backlog-full schedule must drop the job, queued=%d", n0)
	}
	tr.pendingAnnounceMu.Lock()
	n := len(tr.pendingAnnounceJobs)
	tr.pendingAnnounceMu.Unlock()
	if n != MaxPendingAnnounceForwards {
		t.Fatalf("pending jobs = %d, want %d", n, MaxPendingAnnounceForwards)
	}
}

func TestProcessDelayedAnnounceJobs_RunsDueOnly(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	in := newRelayIface("due-in")
	out := newRelayIface("due-out")
	if err := tr.RegisterInterface(in.GetName(), in); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface(out.GetName(), out); err != nil {
		t.Fatal(err)
	}
	// A due job must run (observable as a send on out), a future job must not.
	dueData := []byte{0x04, 0x01}
	var dueDst [16]byte
	futData := []byte{0x04, 0x01}
	var futDst [16]byte
	futDst[15] = 0xFF
	tr.pendingAnnounceMu.Lock()
	tr.pendingAnnounceJobs = []delayedAnnounceJob{
		{due: time.Now().Add(-time.Millisecond), data: dueData, dest: destKey(dueDst[:]), dst: dueDst, from: in},
		{due: time.Now().Add(time.Hour), data: futData, dest: destKey(futDst[:]), dst: futDst, from: in},
	}
	tr.pendingAnnounceMu.Unlock()

	tr.processDelayedAnnounceJobs()
	if countSends(out) == 0 {
		t.Fatal("due job must run")
	}
	tr.pendingAnnounceMu.Lock()
	remaining := len(tr.pendingAnnounceJobs)
	tr.pendingAnnounceMu.Unlock()
	if remaining != 1 {
		t.Fatalf("pending after process = %d, want 1", remaining)
	}
	tr.pendingAnnounceMu.Lock()
	n := len(tr.pendingAnnounceJobs)
	tr.pendingAnnounceMu.Unlock()
	if n != 1 {
		t.Fatalf("pending after process = %d, want 1", n)
	}
}

// Regression: delayed announce forward must not retain slices into the HDLC
// reuse buffer. Under load HandlePacket may dispatch synchronously, return,
// then the decoder overwrites the same backing array.
func TestRegression_AnnounceForwardSurvivesCallerBufferReuse(t *testing.T) {
	useFastPathfinder(t)
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	in := newRelayIface("fwd-in")
	out := newRelayIface("fwd-out")
	if err := tr.RegisterInterface(in.GetName(), in); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface(out.GetName(), out); err != nil {
		t.Fatal(err)
	}

	for i := range 64 {
		buf := bytes.Repeat([]byte{byte(i + 1)}, 48)
		dest := append([]byte(nil), buf[:16]...)
		tr.scheduleAnnounceForward(buf, destKey(dest), dest, in)
		for j := range buf {
			buf[j] = 0xFF
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		tr.processDelayedAnnounceJobs()
		if countSends(out) > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected forwarded announce on out, got %d sends", countSends(out))
}

func TestAnnounceForwardStorm_NoGoroutineExplosion(t *testing.T) {
	useFastPathfinder(t)
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	in := newRelayIface("storm-in")
	out := newRelayIface("storm-out")
	if err := tr.RegisterInterface(in.GetName(), in); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface(out.GetName(), out); err != nil {
		t.Fatal(err)
	}

	before := countSends(out)
	for i := range MaxPendingAnnounceForwards * 2 {
		buf := bytes.Repeat([]byte{byte(i)}, 32)
		dest := append([]byte(nil), randomDestHash(300+i)...)
		tr.scheduleAnnounceForward(buf, destKey(dest), dest, in)
	}
	tr.pendingAnnounceMu.Lock()
	queued := len(tr.pendingAnnounceJobs)
	tr.pendingAnnounceMu.Unlock()
	if queued > MaxPendingAnnounceForwards {
		t.Fatalf("queued %d exceeds cap %d", queued, MaxPendingAnnounceForwards)
	}
	for range 5 {
		tr.processDelayedAnnounceJobs()
	}
	if countSends(out) <= before {
		t.Fatal("storm must still forward some announces")
	}
}
