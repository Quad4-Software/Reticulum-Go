// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package health

import "testing"

func TestDropKindNames(t *testing.T) {
	cases := map[Kind]string{
		KindAnnounceDup:           "announce_dup",
		KindPathRespSuppressed:    "path_resp_suppressed",
		KindPathReqDup:            "path_req_dup",
		KindPathReqNoCache:        "path_req_no_cache",
		KindPathRespQueuedSkip:    "path_resp_queued_skip",
		KindLinkRelayUnknownIface: "link_relay_unknown_iface",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Fatalf("%v got %s want %s", k, got, want)
		}
	}
}

func TestDropCounterSnapshot(t *testing.T) {
	r := NewRegistry()
	r.Inc("udp0", KindAnnounceDup)
	r.Inc("udp0", KindPathRespSuppressed)
	r.Inc("", KindLinkRelayUnknownIface)
	s := r.SnapshotIface("udp0")
	if s.AnnounceDup.Total != 1 || s.PathRespSuppressed.Total != 1 {
		t.Fatalf("iface snap %#v", s)
	}
	tr := r.SnapshotTransport()
	if tr.AnnounceDup.Total != 1 || tr.LinkRelayUnknownIface.Total != 1 {
		t.Fatalf("transport snap %#v", tr)
	}
}
