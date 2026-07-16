// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

type cancelProbeTransport struct{}

func (c *cancelProbeTransport) PrepareFreshPathRequest([]byte) transport.PrepareFreshPathReturn {
	return transport.PrepareFreshNewPathRequested
}

func (c *cancelProbeTransport) ExpirePath([]byte) {}

func TestCancelAllReconnectsStopsLoop(t *testing.T) {
	SetGlobalPaused(false)
	defer SetGlobalPaused(false)

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr := transport.NewTransport(nil)
	defer func() { _ = tr.Close() }()

	iface := newNoopIface("cancel-test")
	iface.Enable()
	if err := tr.RegisterInterface("cancel-test", iface); err != nil {
		t.Fatal(err)
	}

	dest, err := destination.New(id, destination.Out, destination.Single, "test", tr, "cancel")
	if err != nil {
		t.Fatal(err)
	}
	l := NewLink(dest, tr, iface, nil, nil)
	l.status.Store(int32(StatusClosed))

	probe := &cancelProbeTransport{}
	done := make(chan struct{})
	go func() {
		reestablishLink(l, probe, ReconnectPolicy{MaxAttempts: 0, Backoff: 20 * time.Millisecond})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	CancelAllReconnects()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reestablishLink did not stop after CancelAllReconnects")
	}
}

func TestSetGlobalPausedCancelsReconnects(t *testing.T) {
	SetGlobalPaused(false)
	defer SetGlobalPaused(false)

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr := transport.NewTransport(nil)
	defer func() { _ = tr.Close() }()

	iface := newNoopIface("pause-test")
	iface.Enable()
	if err := tr.RegisterInterface("pause-test", iface); err != nil {
		t.Fatal(err)
	}

	dest, err := destination.New(id, destination.Out, destination.Single, "test", tr, "pause")
	if err != nil {
		t.Fatal(err)
	}
	l := NewLink(dest, tr, iface, nil, nil)
	l.status.Store(int32(StatusClosed))

	probe := &cancelProbeTransport{}
	done := make(chan struct{})
	go func() {
		reestablishLink(l, probe, ReconnectPolicy{MaxAttempts: 0, Backoff: 20 * time.Millisecond})
		close(done)
	}()

	time.Sleep(40 * time.Millisecond)
	SetGlobalPaused(true)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reestablishLink did not stop after SetGlobalPaused")
	}
}
