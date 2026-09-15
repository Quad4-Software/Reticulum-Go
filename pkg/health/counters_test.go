// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package health

import "testing"

func TestCounterSnapshotDeltaAndIntegrityFails(t *testing.T) {
	before := CounterSnapshot{RxOK: 10, UnpackFail: 1, AnnounceOK: 4}
	after := CounterSnapshot{RxOK: 25, UnpackFail: 5, AnnounceOK: 9, HMACFail: 2}
	d := before.Delta(after)
	if d.RxOK != 15 || d.UnpackFail != 4 || d.AnnounceOK != 5 || d.HMACFail != 2 {
		t.Fatalf("unexpected delta: %+v", d)
	}
	if d.IntegrityFails() != 6 {
		t.Fatalf("IntegrityFails=%d want 6", d.IntegrityFails())
	}
}

func TestTransportCountersReflectsInc(t *testing.T) {
	r := NewRegistry()
	before := r.TransportCounters()
	r.Inc("", KindRxOK)
	r.Inc("", KindUnpackFail)
	r.Inc("", KindAnnounceOK)
	r.Inc("", KindKeepaliveTimeout)
	r.Inc("", KindLinkStaleClose)
	d := before.Delta(r.TransportCounters())
	if d.RxOK != 1 || d.UnpackFail != 1 || d.AnnounceOK != 1 {
		t.Fatalf("oracle delta=%+v", d)
	}
	if d.KeepaliveTimeout != 1 || d.LinkStaleClose != 1 {
		t.Fatalf("keepalive/stale oracle delta=%+v", d)
	}
}
