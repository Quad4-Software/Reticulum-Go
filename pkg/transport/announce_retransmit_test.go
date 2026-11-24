// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
)

func countJobSends(iface *relayIface) int {
	return len(iface.snapshot())
}

func pendingJobCount(tr *Transport) int {
	tr.pendingAnnounceMu.Lock()
	defer tr.pendingAnnounceMu.Unlock()
	return len(tr.pendingAnnounceJobs)
}

func getJob(t *testing.T, tr *Transport, idx int) *delayedAnnounceJob {
	t.Helper()
	tr.pendingAnnounceMu.Lock()
	defer tr.pendingAnnounceMu.Unlock()
	if idx >= len(tr.pendingAnnounceJobs) {
		t.Fatalf("job index %d out of range (%d jobs)", idx, len(tr.pendingAnnounceJobs))
	}
	return &tr.pendingAnnounceJobs[idx]
}

// Python announce_table: a queued announce emits at the random retransmit
// timeout, retries until retries > PATHFINDER_R, then completes.
func TestDelayedAnnounceJob_RetransmitSequence(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer func() { _ = tr.Close() }()

	in := newRelayIface("rtx-in")
	out := newRelayIface("rtx-out")
	if err := tr.RegisterInterface(in.GetName(), in); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface(out.GetName(), out); err != nil {
		t.Fatal(err)
	}

	var dst [16]byte
	dst[0] = 0xAB
	tr.pendingAnnounceMu.Lock()
	tr.pendingAnnounceJobs = []delayedAnnounceJob{
		{due: time.Now().Add(-time.Millisecond), data: []byte{0x04, 0x03}, dest: destKey(dst[:]), dst: dst, from: in},
	}
	tr.pendingAnnounceMu.Unlock()

	// First emit: retries 0->1, retransmit timeout moves to PATHFINDER_G+RW.
	tr.processDelayedAnnounceJobs()
	if got := countJobSends(out); got != 1 {
		t.Fatalf("first emit sends = %d, want 1", got)
	}
	job := getJob(t, tr, 0)
	if job.retries != 1 {
		t.Fatalf("retries after first emit = %d, want 1", job.retries)
	}
	until := time.Until(job.due)
	if until < 4*time.Second || until > 7*time.Second {
		t.Fatalf("retransmit timeout = %v, want ~PATHFINDER_G+PATHFINDER_RW", until)
	}

	// Second emit at the retry timeout: retries 1->2.
	getJob(t, tr, 0).due = time.Now().Add(-time.Millisecond)
	tr.processDelayedAnnounceJobs()
	if got := countJobSends(out); got != 2 {
		t.Fatalf("second emit sends = %d, want 2", got)
	}
	if getJob(t, tr, 0).retries != 2 {
		t.Fatalf("retries = %d, want 2", getJob(t, tr, 0).retries)
	}

	// retries > PATHFINDER_R completes the entry without another emit.
	getJob(t, tr, 0).due = time.Now().Add(-time.Millisecond)
	tr.processDelayedAnnounceJobs()
	if got := countJobSends(out); got != 2 {
		t.Fatalf("completed job emitted again, sends = %d", got)
	}
	if n := pendingJobCount(tr); n != 0 {
		t.Fatalf("job not completed, %d pending", n)
	}
}

// Python: a local-client announce emits immediately and only once
// (retries starts at PATHFINDER_R).
func TestDelayedAnnounceJob_LocalClientEmitsOnce(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer func() { _ = tr.Close() }()

	in := newRelayIface("lc-in")
	out := newRelayIface("lc-out")
	if err := tr.RegisterInterface(in.GetName(), in); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface(out.GetName(), out); err != nil {
		t.Fatal(err)
	}

	var dst [16]byte
	dst[0] = 0xCD
	tr.scheduleAnnounceForward([]byte{0x04, 0x00}, destKey(dst[:]), dst[:], in, 0, true)
	job := getJob(t, tr, 0)
	if job.retries != PathfinderR {
		t.Fatalf("local-client retries = %d, want PATHFINDER_R", job.retries)
	}
	if job.due.After(time.Now().Add(100 * time.Millisecond)) {
		t.Fatal("local-client job must be due immediately")
	}

	tr.processDelayedAnnounceJobs()
	if got := countJobSends(out); got != 1 {
		t.Fatalf("local-client emit sends = %d, want 1", got)
	}
	getJob(t, tr, 0).due = time.Now().Add(-time.Millisecond)
	tr.processDelayedAnnounceJobs()
	if got := countJobSends(out); got != 1 {
		t.Fatalf("local-client announce emitted twice, sends = %d", got)
	}
	if n := pendingJobCount(tr); n != 0 {
		t.Fatalf("local-client job not completed, %d pending", n)
	}
}

// Python: hearing the same emission rebroadcast back at our hop count counts
// toward LOCAL_REBROADCASTS_MAX. Hearing it passed on (hops+1) cancels retries.
func TestHeardAnnounceRebroadcast(t *testing.T) {
	newJob := func(retries int, hops byte) (tr *Transport, dst [16]byte) {
		tr = NewTransport(&common.ReticulumConfig{EnableTransport: true})
		dst[0] = 0xEF
		tr.pendingAnnounceMu.Lock()
		tr.pendingAnnounceJobs = []delayedAnnounceJob{
			{due: time.Now().Add(time.Hour), data: []byte{0x04, hops}, dest: destKey(dst[:]), dst: dst, retries: retries},
		}
		tr.pendingAnnounceMu.Unlock()
		return tr, dst
	}

	t.Run("counts same-emission rebroadcasts and completes at max", func(t *testing.T) {
		tr, dst := newJob(1, 3)
		defer func() { _ = tr.Close() }()

		tr.heardAnnounceRebroadcast(destKey(dst[:]), 3)
		if n := pendingJobCount(tr); n != 1 {
			t.Fatalf("job dropped after first heard rebroadcast, %d pending", n)
		}
		if rb := getJob(t, tr, 0).localRebroadcasts; rb != 1 {
			t.Fatalf("localRebroadcasts = %d, want 1", rb)
		}

		tr.heardAnnounceRebroadcast(destKey(dst[:]), 3)
		if n := pendingJobCount(tr); n != 0 {
			t.Fatalf("job not completed at local rebroadcast max, %d pending", n)
		}
	})

	t.Run("cancels when our rebroadcast is passed on", func(t *testing.T) {
		tr, dst := newJob(1, 3)
		defer func() { _ = tr.Close() }()

		tr.heardAnnounceRebroadcast(destKey(dst[:]), 4)
		if n := pendingJobCount(tr); n != 0 {
			t.Fatalf("job not cancelled when passed on, %d pending", n)
		}
	})

	t.Run("no cancellation before first emit", func(t *testing.T) {
		tr, dst := newJob(0, 3)
		defer func() { _ = tr.Close() }()

		tr.heardAnnounceRebroadcast(destKey(dst[:]), 4)
		if n := pendingJobCount(tr); n != 1 {
			t.Fatalf("pending job cancelled before first emit, %d pending", n)
		}
	})
}

// Python table_insertion_only: a same-emission announce that only replaces an
// unresponsive path is not queued for retransmission.
func TestAnnouncePathDecision_InsertionOnly(t *testing.T) {
	mkBlob := func(emitted uint32) []byte {
		b := make([]byte, 10)
		for i := range 5 {
			b[5+i] = byte(emitted >> (8 * (4 - i)))
		}
		return b
	}
	existing := &common.Path{
		HopCount:    1,
		RandomBlobs: [][]byte{mkBlob(1000)},
	}

	// Higher hops, same emission timebase, unresponsive path: wins but is
	// insertion-only.
	add, insOnly := announcePathDecision(existing, announcePathInput{
		destinationKnown: true,
		announceHops:     2,
		randomBlob:       mkBlob(1000),
		now:              time.Now(),
	}, true)
	if !add || !insOnly {
		t.Fatalf("same-emission unresponsive replace: add=%v insertionOnly=%v, want true,true", add, insOnly)
	}

	// Higher hops, newer emission: wins and retransmits.
	add, insOnly = announcePathDecision(existing, announcePathInput{
		destinationKnown: true,
		announceHops:     2,
		randomBlob:       mkBlob(2000),
		now:              time.Now(),
	}, true)
	if !add || insOnly {
		t.Fatalf("newer emission replace: add=%v insertionOnly=%v, want true,false", add, insOnly)
	}

	// Higher hops, same emission, responsive path: loses.
	add, insOnly = announcePathDecision(existing, announcePathInput{
		destinationKnown: true,
		announceHops:     2,
		randomBlob:       mkBlob(1000),
		now:              time.Now(),
	}, false)
	if add || insOnly {
		t.Fatalf("responsive same-emission: add=%v insertionOnly=%v, want false,false", add, insOnly)
	}
}

// Python evaluates the heard-rebroadcast rules on announce_table membership,
// which in Go also covers path-response entries in announceTable, so their
// hop patterns must pass the pre-verify dedup bypass too.
func TestAnnounceHeardMatch_AnnounceTableEntry(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer func() { _ = tr.Close() }()

	var dst [16]byte
	dst[0] = 0xC4
	key := destKey(dst[:])

	tr.mutex.Lock()
	tr.announceTable[key] = &PathAnnounceEntry{
		AnnounceHops:      3,
		Retries:           PathfinderR,
		RetransmitTimeout: time.Now().Add(PathRequestGrace),
	}
	tr.mutex.Unlock()

	if !tr.announceHeardMatch(key, 3) {
		t.Fatal("same-emission hops did not match announce-table entry")
	}
	if !tr.announceHeardMatch(key, 4) {
		t.Fatal("passed-on hops did not match announce-table entry")
	}
	if tr.announceHeardMatch(key, 5) {
		t.Fatal("unrelated hops matched announce-table entry")
	}
	var other [16]byte
	other[0] = 0xC5
	if tr.announceHeardMatch(destKey(other[:]), 3) {
		t.Fatal("matched entry for a different destination")
	}
}
