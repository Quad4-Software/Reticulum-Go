// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
)

// fakeAwareDriver implements AwareDriver for tests. It records sends and
// drives events synchronously.
type fakeAwareDriver struct {
	mu       sync.Mutex
	events   AwareEvents
	sends    [][]byte
	closed   []int
	started  string
	stopped  bool
	sendErr  error
	sendHook func(peerID int)
}

func (f *fakeAwareDriver) StartPublish() error {
	f.mu.Lock()
	f.started = "publish"
	f.mu.Unlock()
	return nil
}
func (f *fakeAwareDriver) StartSubscribe() error {
	f.mu.Lock()
	f.started = "subscribe"
	f.mu.Unlock()
	return nil
}
func (f *fakeAwareDriver) Stop() error { f.mu.Lock(); f.stopped = true; f.mu.Unlock(); return nil }
func (f *fakeAwareDriver) SetEvents(e AwareEvents) {
	f.mu.Lock()
	f.events = e
	f.mu.Unlock()
}
func (f *fakeAwareDriver) Send(peerID int, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	cp := append([]byte(nil), data...)
	f.sends = append(f.sends, cp)
	if f.sendHook != nil {
		go f.sendHook(peerID)
	}
	return nil
}
func (f *fakeAwareDriver) ClosePeer(peerID int) error {
	f.mu.Lock()
	f.closed = append(f.closed, peerID)
	f.mu.Unlock()
	return nil
}

func (f *fakeAwareDriver) linkUp(id int) { f.mu.Lock(); e := f.events; f.mu.Unlock(); e.OnLinkUp(id) }
func (f *fakeAwareDriver) linkDown(id int) {
	f.mu.Lock()
	e := f.events
	f.mu.Unlock()
	e.OnLinkDown(id)
}
func (f *fakeAwareDriver) data(id int, d []byte) {
	f.mu.Lock()
	e := f.events
	f.mu.Unlock()
	e.OnData(id, d)
}
func (f *fakeAwareDriver) sessionLost() {
	f.mu.Lock()
	e := f.events
	f.mu.Unlock()
	e.OnSessionLost()
}

func TestAwareInterfaceStartRoles(t *testing.T) {
	d := &fakeAwareDriver{}
	ai, err := NewAwareInterface("wifi-aware", "publish", 4, d, nil)
	if err != nil {
		t.Fatalf("NewAwareInterface: %v", err)
	}
	if err := ai.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if d.started != "publish" {
		t.Fatalf("driver role=%q want publish", d.started)
	}
	if err := ai.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !d.stopped {
		t.Fatal("driver not stopped")
	}
}

func TestAwareInterfaceRequiresDriver(t *testing.T) {
	ai, err := NewAwareInterface("wifi-aware", "", 0, nil, nil)
	if err != nil {
		t.Fatalf("NewAwareInterface: %v", err)
	}
	if ai.role != "subscribe" {
		t.Fatalf("default role=%q want subscribe", ai.role)
	}
	if err := ai.Start(); err == nil {
		t.Fatal("Start must fail without a driver")
	}
	if _, err := NewAwareInterface("wifi-aware", "bogus", 0, &fakeAwareDriver{}, nil); err == nil {
		t.Fatal("invalid role must fail")
	}
}

func TestAwarePeerSpawnAndCap(t *testing.T) {
	d := &fakeAwareDriver{}
	var spawned []*AwarePeerInterface
	ai, err := NewAwareInterface("wifi-aware", "subscribe", 2, d, func(p *AwarePeerInterface) {
		spawned = append(spawned, p)
	})
	if err != nil {
		t.Fatalf("NewAwareInterface: %v", err)
	}
	if err := ai.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer ai.Stop()

	d.linkUp(11)
	d.linkUp(12)
	if ai.PeerCount() != 2 {
		t.Fatalf("peers=%d want 2", ai.PeerCount())
	}
	if len(spawned) != 2 {
		t.Fatalf("spawned=%d want 2", len(spawned))
	}

	// Third link exceeds the cap: driver gets a ClosePeer, no spawn.
	d.linkUp(13)
	if ai.PeerCount() != 2 {
		t.Fatalf("cap exceeded: peers=%d", ai.PeerCount())
	}
	d.mu.Lock()
	closedCount := 0
	for _, c := range d.closed {
		if c == 13 {
			closedCount++
		}
	}
	d.mu.Unlock()
	if closedCount == 0 {
		t.Fatal("over-cap peer was not closed by driver")
	}
}

func TestAwarePeerHDLCRoundTrip(t *testing.T) {
	d := &fakeAwareDriver{}
	ai, err := NewAwareInterface("wifi-aware", "subscribe", 4, d, nil)
	if err != nil {
		t.Fatalf("NewAwareInterface: %v", err)
	}
	if err := ai.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer ai.Stop()

	d.linkUp(7)
	ai.Mutex.RLock()
	peer := ai.peers[7]
	ai.Mutex.RUnlock()
	if peer == nil {
		t.Fatal("peer 7 not spawned")
	}

	// Inbound: HDLC frame on the socket becomes a packet through the
	// interface inbound path.
	payload := bytes.Repeat([]byte{0x42}, 40)
	d.data(7, appendFrameHDLC(nil, payload))
	if peer.RxPackets != 1 {
		t.Fatalf("rx packets=%d want 1", peer.RxPackets)
	}

	// Outbound: ProcessOutgoing frames identically to TCP.
	if err := peer.ProcessOutgoing(payload); err != nil {
		t.Fatalf("ProcessOutgoing: %v", err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.sends) != 1 {
		t.Fatalf("sends=%d want 1", len(d.sends))
	}
	want := appendFrameHDLC(nil, payload)
	if !bytes.Equal(d.sends[0], want) {
		t.Fatalf("framing mismatch: got %x want %x", d.sends[0][:16], want[:16])
	}
}

func TestAwarePeerSendErrorTearsDown(t *testing.T) {
	d := &fakeAwareDriver{sendErr: errors.New("socket gone")}
	ai, err := NewAwareInterface("wifi-aware", "subscribe", 4, d, nil)
	if err != nil {
		t.Fatalf("NewAwareInterface: %v", err)
	}
	if err := ai.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer ai.Stop()

	d.linkUp(3)
	ai.Mutex.RLock()
	peer := ai.peers[3]
	ai.Mutex.RUnlock()
	if peer == nil {
		t.Fatal("peer not spawned")
	}
	if err := peer.ProcessOutgoing([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected send error")
	}
	if ai.PeerCount() != 0 {
		t.Fatalf("peer not torn down after send error: %d", ai.PeerCount())
	}
}

func TestAwareLinkDownAndSessionLost(t *testing.T) {
	d := &fakeAwareDriver{}
	ai, err := NewAwareInterface("wifi-aware", "subscribe", 4, d, nil)
	if err != nil {
		t.Fatalf("NewAwareInterface: %v", err)
	}
	if err := ai.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	d.linkUp(1)
	d.linkUp(2)
	d.linkDown(1)
	if ai.PeerCount() != 1 {
		t.Fatalf("peers=%d want 1 after link_down", ai.PeerCount())
	}

	d.sessionLost()
	if ai.PeerCount() != 0 {
		t.Fatalf("session_lost did not clear peers: %d", ai.PeerCount())
	}
	if !d.stopped {
		t.Fatal("session_lost did not stop driver")
	}
}

func TestAwareRestartRecreatesSession(t *testing.T) {
	d := &fakeAwareDriver{}
	ai, err := NewAwareInterface("wifi-aware", "subscribe", 4, d, nil)
	if err != nil {
		t.Fatalf("NewAwareInterface: %v", err)
	}
	if err := ai.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	d.linkUp(5)
	if err := ai.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if ai.PeerCount() != 0 {
		t.Fatal("stop did not clear peers")
	}

	// Restart must be possible (done channel recreation).
	d.stopped = false
	if err := ai.Start(); err != nil {
		t.Fatalf("restart Start: %v", err)
	}
	d.linkUp(9)
	if ai.PeerCount() != 1 {
		t.Fatalf("restart did not spawn peer: %d", ai.PeerCount())
	}
	defer ai.Stop()
}

func TestAwareFromConfig(t *testing.T) {
	d := &fakeAwareDriver{}
	cfg := &common.InterfaceConfig{
		Type:       "AwareInterface",
		Enabled:    true,
		AwareRole:  "publish",
		AwarePeers: 3,
	}
	var spawned []*AwarePeerInterface
	ctx := &FromConfigContext{
		AwareDriver: d,
		SpawnAware: func(p *AwarePeerInterface) {
			spawned = append(spawned, p)
		},
	}
	iface, err := NewFromConfigWithContext("wifi-aware", cfg, ctx)
	if err != nil {
		t.Fatalf("NewFromConfigWithContext: %v", err)
	}
	if err := iface.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	d.linkUp(1)
	if len(spawned) != 1 {
		t.Fatalf("spawn hook not invoked: %d", len(spawned))
	}
	defer iface.Stop()
}
