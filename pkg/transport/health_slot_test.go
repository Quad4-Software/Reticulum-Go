// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/health"
)

// Churned spawned interfaces must not leave health slots behind: Python
// keeps per-interface counters on the interface object, so they die with
// it. UnregisterInterface is the Go-side death point.
func TestUnregisterInterfaceDropsHealthSlot(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer func() { _ = tr.Close() }()
	defer health.Default.Reset()
	health.Default.Reset()

	iface := newSimIface("churned-iface")
	if err := tr.RegisterInterface(iface.GetName(), iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}
	health.Inc(iface.GetName(), health.KindAnnounceOK)

	before := health.Default.IfaceCount()
	tr.UnregisterInterface(iface.GetName())
	if got := health.Default.IfaceCount(); got != before-1 {
		t.Fatalf("IfaceCount = %d after unregister, want %d", got, before-1)
	}
	if s := health.Default.SnapshotIface(iface.GetName()); s.AnnounceOK.Total != 0 {
		t.Fatalf("iface slot still readable after unregister: %+v", s)
	}
}
