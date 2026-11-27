// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package health

import "testing"

// Spawned interfaces get per-connection names ("Client on X@ip:port"), so a
// health slot that outlives its interface grows byIface without bound on
// connection churn. The transport drops the slot when the interface is
// unregistered. This test pins the registry-level removal primitive.
func TestRemoveIfaceDropsSlot(t *testing.T) {
	r := NewRegistry()
	r.Inc("spawned@10.0.0.1:1234", KindAnnounceOK)
	if r.IfaceCount() != 1 {
		t.Fatalf("IfaceCount = %d, want 1", r.IfaceCount())
	}
	r.RemoveIface("spawned@10.0.0.1:1234")
	if r.IfaceCount() != 0 {
		t.Fatalf("IfaceCount = %d, want 0 after RemoveIface", r.IfaceCount())
	}
	// Removing an unknown name is a no-op.
	r.RemoveIface("spawned@10.0.0.1:1234")
	// Transport totals keep their counts. Only the iface slot is dropped.
	if got := r.SnapshotTransport().AnnounceOK.Total; got != 1 {
		t.Fatalf("transport AnnounceOK = %d, want 1", got)
	}
}
