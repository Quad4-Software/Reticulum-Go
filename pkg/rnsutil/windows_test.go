// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"context"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/transport"
)

func TestPathResponseWindowMatchesTransport(t *testing.T) {
	tr := transport.NewTransport(nil)
	dest := make([]byte, 16)
	got := PathResponseWindow(tr, dest)
	want := tr.PathResponseWindow(dest)
	if got != want {
		t.Fatalf("PathResponseWindow = %s, want %s", got, want)
	}
}

func TestBoundWaitInheritsDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	child, stop := BoundWait(parent, time.Hour)
	defer stop()
	deadline, ok := child.Deadline()
	if !ok {
		t.Fatal("child should inherit parent deadline")
	}
	parentDeadline, _ := parent.Deadline()
	if !deadline.Equal(parentDeadline) {
		t.Fatalf("child deadline %v, parent %v", deadline, parentDeadline)
	}
}

func TestBoundWaitAppliesWindowWithoutDeadline(t *testing.T) {
	child, stop := BoundWait(context.Background(), 40*time.Millisecond)
	defer stop()
	select {
	case <-child.Done():
	case <-time.After(time.Second):
		t.Fatal("window should have expired")
	}
}

func TestLinkEstablishmentWindowAddsMargin(t *testing.T) {
	l := link.NewLink(nil, nil, nil, nil, nil)
	got := LinkEstablishmentWindow(l)
	want := l.EstablishmentTimeout() + LinkEstablishmentMargin
	if got != want {
		t.Fatalf("link window = %s, want %s", got, want)
	}
}

func TestSlowestOnlineBitrateFromStats(t *testing.T) {
	stats := transport.InterfaceStatsResponse{
		Interfaces: []transport.InterfaceStat{
			{Status: false, Bitrate: 50},
			{Status: true, Bitrate: 10_000_000},
			{Status: true, Bitrate: 125},
		},
	}
	if got := slowestOnlineBitrateFromStats(stats); got != 125 {
		t.Fatalf("slowest = %d, want 125", got)
	}
}
