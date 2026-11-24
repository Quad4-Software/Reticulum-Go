// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

func TestReestablishFromClosed(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr := transport.NewTransport(nil)
	defer func() { _ = tr.Close() }()

	iface := newNoopIface("test")
	iface.Enable()
	if err := tr.RegisterInterface("test", iface); err != nil {
		t.Fatal(err)
	}

	dest, err := destination.New(id, destination.Out, destination.Single, "test", tr, "link")
	if err != nil {
		t.Fatal(err)
	}
	destHash := dest.GetHash()
	tr.UpdatePath(destHash, destHash, "test", 1)

	l := NewLink(dest, tr, iface, nil, nil)
	l.status.Store(int32(StatusClosed))
	if err := l.Reestablish(); err != nil {
		t.Fatalf("Reestablish: %v", err)
	}
	if l.GetStatus() != StatusPending {
		t.Fatalf("status = %d, want pending", l.GetStatus())
	}
	if len(l.linkID) == 0 {
		t.Fatal("expected linkID after reestablish")
	}
}
