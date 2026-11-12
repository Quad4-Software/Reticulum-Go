// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
)

type statsQueueIface struct {
	interfaces.BaseInterface
	sent [][]byte
}

func (s *statsQueueIface) Send(data []byte, address string) error {
	s.sent = append(s.sent, append([]byte(nil), data...))
	return nil
}

func (s *statsQueueIface) ProcessOutgoing(data []byte) error {
	return s.Send(data, "")
}

// statsBlockingIface parks inside GetRxBytes so a test can hold the
// transport read lock mid-snapshot while a writer queues behind it.
type statsBlockingIface struct {
	mockInterface
	entered chan struct{}
	unblock chan struct{}
}

func (s *statsBlockingIface) GetRxBytes() uint64 {
	select {
	case s.entered <- struct{}{}:
	default:
	}
	<-s.unblock
	return 0
}

func TestGetInterfaceStatsRPCTypeNames(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	t.Cleanup(func() { _ = tr.Close() })

	udp, err := interfaces.NewUDPInterface("udp-stats", "127.0.0.1:0", "", true)
	if err != nil {
		t.Fatal(err)
	}
	tcp, err := interfaces.NewTCPClientInterface("tcp-stats", "127.0.0.1", 1, false, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface(udp.GetName(), udp); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface(tcp.GetName(), tcp); err != nil {
		t.Fatal(err)
	}

	stats := tr.GetInterfaceStatsRPC()
	got := map[string]string{}
	for i := range stats.Interfaces {
		got[stats.Interfaces[i].Name] = stats.Interfaces[i].Type
		if stats.Interfaces[i].Name == udp.GetName() && stats.Interfaces[i].AnnounceQueue != 0 {
			t.Fatalf("empty UDP queue want 0 got %d", stats.Interfaces[i].AnnounceQueue)
		}
	}
	if got[udp.GetName()] != "UDPInterface" {
		t.Fatalf("UDP type=%q want UDPInterface", got[udp.GetName()])
	}
	if got[tcp.GetName()] != "TCPClientInterface" {
		t.Fatalf("TCP type=%q want TCPClientInterface", got[tcp.GetName()])
	}
}

// GetInterfaceStatsRPC must not re-acquire t.mutex through helper calls:
// Go RWMutex read locks are not reentrant once a writer is queued, so a
// nested RLock deadlocks the transport mutex (seen as daemon/reload timeouts
// under qemu-user self-check).
func TestGetInterfaceStatsRPCNoRecursiveReadLock(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	t.Cleanup(func() { _ = tr.Close() })

	blocker := &statsBlockingIface{
		entered: make(chan struct{}, 1),
		unblock: make(chan struct{}),
	}
	blocker.Name = "stats-blocker"
	blocker.Enabled = true
	blocker.Online = true
	if err := tr.RegisterInterface("stats-blocker", blocker); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	statsDone := make(chan InterfaceStatsResponse, 1)
	go func() { statsDone <- tr.GetInterfaceStatsRPC() }()

	select {
	case <-blocker.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("GetInterfaceStatsRPC did not reach interface stats")
	}

	// Queue a writer behind the in-flight read lock.
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- tr.RegisterInterface("stats-writer", mockIface("stats-writer", true))
	}()

	// A pending writer makes TryRLock fail; poll until the writer is queued.
	deadline := time.Now().Add(10 * time.Second)
	for tr.mutex.TryRLock() {
		tr.mutex.RUnlock()
		if time.Now().After(deadline) {
			t.Fatal("writer never queued on transport mutex")
		}
		time.Sleep(time.Millisecond)
	}

	close(blocker.unblock)

	select {
	case <-statsDone:
	case <-time.After(10 * time.Second):
		t.Fatal("GetInterfaceStatsRPC deadlocked behind queued writer")
	}
	if err := <-writerDone; err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}
}

func TestForwardAnnounceQueuesWhenCapped(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	t.Cleanup(func() { _ = tr.Close() })

	in := &statsQueueIface{}
	in.Name = "q-in"
	in.Enabled = true
	in.Online = true
	in.Bitrate = 1200
	in.Mode = common.IFModeFull

	out := &statsQueueIface{}
	out.Name = "q-out"
	out.Enabled = true
	out.Online = true
	out.Bitrate = 1200
	out.Mode = common.IFModeFull

	if err := tr.RegisterInterface(in.GetName(), in); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface(out.GetName(), out); err != nil {
		t.Fatal(err)
	}

	dest := make([]byte, 16)
	dest[0] = 0xab
	pkt := make([]byte, 48)
	copy(pkt[2:], dest)
	pkt[1] = 1

	if err := tr.forwardAnnouncePacket(pkt, destKey(dest), dest, in); err != nil {
		t.Fatal(err)
	}
	if len(out.sent) != 1 {
		t.Fatalf("first forward sent=%d want 1", len(out.sent))
	}

	pkt2 := append([]byte(nil), pkt...)
	pkt2[3] = 0xcd
	if err := tr.forwardAnnouncePacket(pkt2, destKey(dest), dest, in); err != nil {
		t.Fatal(err)
	}
	if len(out.sent) != 1 {
		t.Fatalf("capped forward sent=%d want 1 (rest queued)", len(out.sent))
	}

	stats := tr.GetInterfaceStatsRPC()
	var found *InterfaceStat
	for i := range stats.Interfaces {
		if stats.Interfaces[i].Name == out.GetName() {
			found = &stats.Interfaces[i]
			break
		}
	}
	if found == nil {
		t.Fatal("missing q-out in stats")
	}
	if found.Type != "statsQueueIface" {
		t.Fatalf("type=%q want statsQueueIface", found.Type)
	}
	if found.AnnounceQueue != 1 {
		t.Fatalf("announce_queue=%d want 1", found.AnnounceQueue)
	}

	if n := tr.DropAnnounceQueuesRPC(); n != 1 {
		t.Fatalf("drop=%d want 1", n)
	}
	if out.AnnounceQueueLen() != 0 {
		t.Fatal("queue not empty after drop")
	}
}
