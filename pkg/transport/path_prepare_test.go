// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
)

func TestPrepareFreshPathRequest_ReusesValidPath(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))
	out := newRelayIface("out")
	_ = tr.RegisterInterface("out", out)

	dest := bytes.Repeat([]byte{0x33}, 16)
	tr.UpdatePath(dest, bytes.Repeat([]byte{0x44}, 16), "out", 2)

	if got := tr.PrepareFreshPathRequest(dest); got != PrepareFreshReusedValidPath {
		t.Fatalf("expected reused_valid_path, got %q", got)
	}
	if n := len(out.snapshot()); n != 0 {
		t.Fatalf("expected no path request packet, got %d", n)
	}
}

func TestPrepareFreshPathRequest_NewDestinationEmitsPacket(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))
	out := newRelayIface("out")
	_ = tr.RegisterInterface("out", out)

	dest := bytes.Repeat([]byte{0x66}, 16)
	if got := tr.PrepareFreshPathRequest(dest); got != PrepareFreshNewPathRequested {
		t.Fatalf("expected new_path_requested, got %q", got)
	}
	if n := len(out.snapshot()); n != 1 {
		t.Fatalf("expected one path request packet, got %d", n)
	}
}

func TestPrepareFreshPathRequest_UnresponsiveDropsAndRefreshes(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))
	out := newRelayIface("out")
	_ = tr.RegisterInterface("out", out)

	dest := bytes.Repeat([]byte{0x77}, 16)
	tr.UpdatePath(dest, bytes.Repeat([]byte{0x88}, 16), "out", 2)
	tr.MarkPathUnresponsive(dest)

	before := len(out.snapshot())
	if got := tr.PrepareFreshPathRequest(dest); got != PrepareFreshPathRefreshRequested {
		t.Fatalf("expected path_refresh_requested, got %q", got)
	}
	if len(out.snapshot()) <= before {
		t.Fatal("expected path refresh to emit at least one packet")
	}
}

func TestNudgePathRequest_BypassesThrottle(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))
	out := newRelayIface("out")
	_ = tr.RegisterInterface("out", out)

	dest := bytes.Repeat([]byte{0x55}, 16)
	if err := tr.RequestPath(dest, "out", nil, false); err != nil {
		t.Fatalf("first RequestPath: %v", err)
	}
	first := len(out.snapshot())
	if err := tr.RequestPath(dest, "out", nil, false); err != nil {
		t.Fatalf("second RequestPath: %v", err)
	}
	if len(out.snapshot()) != first {
		t.Fatal("throttle should suppress second nil-tag request")
	}
	if err := tr.NudgePathRequest(dest); err != nil {
		t.Fatalf("NudgePathRequest: %v", err)
	}
	if len(out.snapshot()) <= first {
		t.Fatal("nudge should emit another path request")
	}
}

func TestExpirePath_ClearsHasPath(t *testing.T) {
	tr, iface := newHasPathTransport(t)
	dest := randomHash(t, 16)
	tr.UpdatePath(dest, []byte("next"), iface.Name, 1)
	if !tr.HasPath(dest) {
		t.Fatal("fixture should have path")
	}
	tr.ExpirePath(dest)
	if tr.HasPath(dest) {
		t.Fatal("ExpirePath should clear path cache")
	}
}
