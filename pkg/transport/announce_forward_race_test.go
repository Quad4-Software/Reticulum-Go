// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
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
			due: time.Now().Add(time.Hour),
			job: func() {},
		})
	}
	tr.pendingAnnounceMu.Unlock()

	ran := false
	tr.scheduleAnnounceForwardJob(func() { ran = true })
	if ran {
		t.Fatal("backlog-full schedule must drop the job")
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

	var ranDue, ranFuture bool
	tr.pendingAnnounceMu.Lock()
	tr.pendingAnnounceJobs = []delayedAnnounceJob{
		{due: time.Now().Add(-time.Millisecond), job: func() { ranDue = true }},
		{due: time.Now().Add(time.Hour), job: func() { ranFuture = true }},
	}
	tr.pendingAnnounceMu.Unlock()

	tr.processDelayedAnnounceJobs()
	if !ranDue {
		t.Fatal("due job must run")
	}
	if ranFuture {
		t.Fatal("future job must remain queued")
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
		fwd := append([]byte(nil), buf...)
		fwd[1]++
		destCopy := append([]byte(nil), dest...)
		from := in
		tr.scheduleAnnounceForwardJob(func() {
			_ = tr.forwardAnnouncePacket(fwd, destKey(destCopy), destCopy, from)
		})
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
		fwd := append([]byte(nil), buf...)
		tr.scheduleAnnounceForwardJob(func() {
			_ = tr.forwardAnnouncePacket(fwd, destKey(dest), dest, in)
		})
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
