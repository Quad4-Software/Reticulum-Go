// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestAnnounceQueueMinHopsFirst(t *testing.T) {
	b := NewBaseInterface("q", common.IFTypeUDP, true)
	b.Bitrate = 1_000_000
	destFar := bytes.Repeat([]byte{0x11}, 16)
	destNear := bytes.Repeat([]byte{0x22}, 16)
	b.QueueOutgoingAnnounce([]byte("far"), destFar, 5, 0)
	b.QueueOutgoingAnnounce([]byte("near"), destNear, 1, 0)
	if b.AnnounceQueueLen() != 2 {
		t.Fatalf("len=%d want 2", b.AnnounceQueueLen())
	}
	raw, ok := b.NextOutgoingAnnounce()
	if !ok {
		t.Fatal("expected queued announce")
	}
	if string(raw) != "near" {
		t.Fatalf("got %q want near", raw)
	}
	if b.AnnounceQueueLen() != 1 {
		t.Fatalf("len after pop=%d want 1", b.AnnounceQueueLen())
	}
}

func TestAnnounceQueueSameDestKeepsNewerEmitted(t *testing.T) {
	b := NewBaseInterface("q", common.IFTypeUDP, true)
	b.Bitrate = 1_000_000
	dest := bytes.Repeat([]byte{0x33}, 16)
	b.QueueOutgoingAnnounce([]byte("old"), dest, 2, 1)
	b.QueueOutgoingAnnounce([]byte("new"), dest, 2, 9)
	if b.AnnounceQueueLen() != 1 {
		t.Fatalf("len=%d want 1", b.AnnounceQueueLen())
	}
	raw, ok := b.NextOutgoingAnnounce()
	if !ok || string(raw) != "new" {
		t.Fatalf("got %q ok=%v want new", raw, ok)
	}
}

func TestAnnounceQueueDropsStale(t *testing.T) {
	b := NewBaseInterface("q", common.IFTypeUDP, true)
	b.Bitrate = 1_000_000
	dest := bytes.Repeat([]byte{0x44}, 16)
	b.QueueOutgoingAnnounce([]byte("stale"), dest, 1, 0)
	b.Mutex.Lock()
	b.announceQueue[0].queued = time.Now().Add(-QueuedAnnounceLife - time.Second)
	b.Mutex.Unlock()
	if _, ok := b.NextOutgoingAnnounce(); ok {
		t.Fatal("stale announce must be dropped")
	}
	if b.AnnounceQueueLen() != 0 {
		t.Fatalf("len=%d want 0", b.AnnounceQueueLen())
	}
}

func TestAnnounceQueueRespectsAllowedAt(t *testing.T) {
	b := NewBaseInterface("q", common.IFTypeUDP, true)
	b.Bitrate = 1200
	dest := bytes.Repeat([]byte{0x55}, 16)
	b.NoteAnnounceSent(200)
	b.QueueOutgoingAnnounce([]byte("wait"), dest, 1, 0)
	if _, ok := b.NextOutgoingAnnounce(); ok {
		t.Fatal("must wait for announce_cap window")
	}
	if n := b.DropAnnounceQueue(); n != 1 {
		t.Fatalf("drop=%d want 1", n)
	}
	if b.AnnounceQueueLen() != 0 {
		t.Fatal("queue not empty after drop")
	}
}

func TestAnnounceQueueCap(t *testing.T) {
	b := NewBaseInterface("q", common.IFTypeUDP, true)
	b.Bitrate = 1_000_000
	for i := range MaxQueuedAnnounces + 8 {
		dest := make([]byte, 16)
		dest[0] = byte(i)
		dest[1] = byte(i >> 8)
		b.QueueOutgoingAnnounce([]byte{byte(i)}, dest, 1, 0)
	}
	if b.AnnounceQueueLen() != MaxQueuedAnnounces {
		t.Fatalf("len=%d want %d", b.AnnounceQueueLen(), MaxQueuedAnnounces)
	}
}
