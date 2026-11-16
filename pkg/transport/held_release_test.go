// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

// Held announces answering a destination with a pending local path request
// release ahead of unrelated held announces, even when they sit behind
// them in the FIFO queue and the release interval would block a second
// FIFO pop.
func TestHeldAnnouncePriorityRelease(t *testing.T) {
	enableSimFastPath(t)
	inCfg := &common.InterfaceConfig{
		IngressControlSet:     true,
		IngressControl:        true,
		ICBurstFreq:           10,
		ICBurstFreqNew:        2,
		ICNewTime:             30,
		ICBurstHold:           1,
		ICBurstPenalty:        1,
		ICMaxHeldAnnounces:    64,
		ICHeldReleaseInterval: 3600,
	}
	tr, _, out := transportWithIfaceConfig(t, inCfg, nil)
	defer tr.Close()

	st := tr.ifaceStates.get("in")
	if st == nil || st.ingress == nil {
		t.Fatal("missing ingress control")
	}

	// Force the burst state with dummy non-new-destination arrivals.
	for i := 0; i < 100; i++ {
		var h [32]byte
		h[0] = byte(i)
		h[1] = byte(i >> 8)
		st.ingress.ProcessAnnounceHashDest(h, nil, false, [16]byte{})
	}
	if !st.ingress.InBurst() {
		t.Skip("ingress burst did not engage")
	}

	// Queue three real announces for new destinations. All are held.
	// Order in the FIFO queue: head (unrelated), middle (unrelated),
	// tail (the one a local path request is waiting on).
	mkAnnounce := func() ([]byte, []byte, [32]byte, [16]byte) {
		id, err := identity.New()
		if err != nil {
			t.Fatalf("identity.New: %v", err)
		}
		raw, dh := signedAnnounce(t, tr, id)
		return raw, dh, sha256.Sum256(raw), destKey(dh).bytes
	}
	rawHead, _, hHead, dHead := mkAnnounce()
	rawMid, _, hMid, dMid := mkAnnounce()
	rawPend, destPend, hPend, dPend := mkAnnounce()

	for _, q := range []struct {
		hash [32]byte
		data []byte
		dest [16]byte
	}{
		{hHead, rawHead, dHead},
		{hMid, rawMid, dMid},
		{hPend, rawPend, dPend},
	} {
		if st.ingress.ProcessAnnounceHashDest(q.hash, q.data, true, q.dest) {
			t.Fatal("expected announce to be held during burst")
		}
	}
	if got := st.ingress.HeldCount(); got != 3 {
		t.Fatalf("held count = %d, want 3", got)
	}

	waiter := newTrackingIface("waiter")
	if err := tr.RegisterInterface("waiter", waiter); err != nil {
		t.Fatalf("register waiter: %v", err)
	}
	tr.notePendingLocalPathRequest(destPend, waiter)

	// Drive the burst state machine back to clear: sparse dummy arrivals
	// keep frequency under threshold until burstActive drops, then the
	// hold+penalty quiet period (2s) must elapse before releases open.
	deadline := time.Now().Add(8 * time.Second)
	for st.ingress.InBurst() && time.Now().Before(deadline) {
		var h [32]byte
		h[0] = 0xEE
		st.ingress.ProcessAnnounceHashDest(h, nil, false, [16]byte{})
		time.Sleep(150 * time.Millisecond)
	}
	if st.ingress.InBurst() {
		t.Fatal("burst did not clear")
	}
	time.Sleep(2300 * time.Millisecond)

	tr.releaseHeldAnnounces()

	// The pending announce (FIFO tail) must propagate while the unrelated
	// middle entry stays held: the 3600s release interval blocks any
	// further FIFO pops this call.
	pendForwarded := false
	deadline = time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if announceJobPending(tr, dPend) || announceSentContains(out, dPend) {
			pendForwarded = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !pendForwarded {
		t.Fatal("pending-destination held announce was not priority-released")
	}

	// The unrelated middle announce must still be held.
	if announceJobPending(tr, dMid) {
		t.Fatal("unrelated held announce released early")
	}
	if got := st.ingress.HeldCount(); got < 1 {
		t.Fatalf("expected unrelated announce to remain held, held=%d", got)
	}
}

// announceJobPending reports whether a forward job for dst is queued.
func announceJobPending(tr *Transport, dst [16]byte) bool {
	tr.pendingAnnounceMu.Lock()
	defer tr.pendingAnnounceMu.Unlock()
	for _, j := range tr.pendingAnnounceJobs {
		if j.dst == dst {
			return true
		}
	}
	return false
}

// announceSentContains reports whether iface sent a packet containing dst.
func announceSentContains(iface *trackingIface, dst [16]byte) bool {
	iface.mu.Lock()
	defer iface.mu.Unlock()
	for _, pkt := range iface.sent {
		if bytes.Contains(pkt, dst[:]) {
			return true
		}
	}
	return false
}
