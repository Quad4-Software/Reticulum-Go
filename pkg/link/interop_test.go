// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package link

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/destination"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

// PipeInterface simulates a direct connection between two nodes
type PipeInterface struct {
	common.BaseInterface
	peer   *PipeInterface
	tr     *transport.Transport
	online bool
}

func NewPipeInterface(name string) *PipeInterface {
	return &PipeInterface{
		BaseInterface: common.BaseInterface{
			Name:    name,
			Type:    common.IFTypeUDP,
			Enabled: true,
			Online:  true,
		},
		online: true,
	}
}

func (p *PipeInterface) Send(data []byte, address string) error {
	if !p.online || p.peer == nil || !p.peer.online {
		return nil
	}
	// Deliver to peer's transport
	if p.peer.tr != nil {
		dataCopy := make([]byte, len(data))
		copy(dataCopy, data)
		p.peer.tr.HandlePacket(dataCopy, p.peer)
	}
	return nil
}

func (p *PipeInterface) IsEnabled() bool { return p.Enabled }
func (p *PipeInterface) IsOnline() bool  { return p.online }
func (p *PipeInterface) GetName() string { return p.Name }
func (p *PipeInterface) Start() error    { return nil }
func (p *PipeInterface) Stop() error     { return nil }
func (p *PipeInterface) Detach()         {}

func TestNodeInterop(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	// Create Node A
	cfgA := &common.ReticulumConfig{}
	trA := transport.NewTransport(cfgA)
	defer trA.Close()
	idA, _ := identity.New()

	// Create Node B
	cfgB := &common.ReticulumConfig{}
	trB := transport.NewTransport(cfgB)
	defer trB.Close()
	idB, _ := identity.New()
	_ = idB // Use idB to avoid unused error

	// Connect them via PipeInterface
	pipeA := NewPipeInterface("pipeA")
	pipeB := NewPipeInterface("pipeB")
	pipeA.peer = pipeB
	pipeB.peer = pipeA
	pipeA.tr = trA
	pipeB.tr = trB

	_ = trA.RegisterInterface("pipeA", pipeA)
	_ = trB.RegisterInterface("pipeB", pipeB)

	// Initialize path request handler on Node A so it can answer path requests
	_ = trA.InitializePathRequestHandler()

	// Create a destination on Node A
	destA, _ := destination.New(idA, destination.In, destination.Single, "testapp", trA, "service")
	destA.AcceptsLinks(true)

	var wg sync.WaitGroup
	wg.Add(1)

	var establishedLink *Link
	destA.SetLinkEstablishedCallback(func(l any) {
		link, ok := l.(*Link)
		if ok {
			establishedLink = link
			wg.Done()
		}
	})

	// Node A announces itself
	t.Log("Node A announcing...")
	if err := destA.Announce(false, nil, nil); err != nil {
		t.Fatalf("Announce failed: %v", err)
	}

	// Give time for announce to propagate
	time.Sleep(200 * time.Millisecond)

	// Check if Node B saw the announce and has a path
	if !trB.HasPath(destA.GetHash()) {
		t.Error("Node B should have a path to Node A after announce")
	} else {
		t.Logf("Node B has path to Node A: %d hops", trB.HopsTo(destA.GetHash()))
	}

	// Node B establishes a link to Node A
	t.Log("Node B establishing link to Node A...")
	var wgB sync.WaitGroup
	wgB.Add(1)
	linkB := NewLink(destA, trB, pipeB, func(l *Link) {
		wgB.Done()
	}, nil)

	if err := linkB.Establish(); err != nil {
		t.Fatalf("Link establishment failed: %v", err)
	}

	// Wait for link establishment confirmation on Node B
	doneB := make(chan struct{})
	go func() {
		wgB.Wait()
		close(doneB)
	}()

	select {
	case <-doneB:
		t.Log("Link established successfully on Node B")
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for link establishment on Node B")
	}

	// Wait for link establishment confirmation on Node A
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("Link established successfully on Node A")
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for link establishment")
	}

	if establishedLink == nil {
		t.Fatal("Link established but establishedLink is nil")
	}

	// Verify link IDs match
	if !bytes.Equal(linkB.linkID, establishedLink.linkID) {
		t.Errorf("Link ID mismatch: %x != %x", linkB.linkID, establishedLink.linkID)
	}

	// Test bidirectional communication over link
	t.Log("Testing communication over link...")
	msg := []byte("hello from node B")
	var receivedMsg []byte
	var msgWg sync.WaitGroup
	msgWg.Add(1)

	establishedLink.SetPacketCallback(func(data []byte, p *packet.Packet) {
		receivedMsg = data
		msgWg.Done()
	})

	if err := linkB.SendPacket(msg); err != nil {
		t.Fatalf("Failed to send packet over link: %v", err)
	}

	msgDone := make(chan struct{})
	go func() {
		msgWg.Wait()
		close(msgDone)
	}()

	select {
	case <-msgDone:
		if !bytes.Equal(receivedMsg, msg) {
			t.Errorf("Received message mismatch: %q != %q", receivedMsg, msg)
		} else {
			t.Log("Message received successfully over link")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting for message over link")
	}

	// Test identification
	t.Log("Node B identifying to Node A...")
	var identWg sync.WaitGroup
	identWg.Add(1)
	var identifiedRemote *identity.Identity

	establishedLink.SetRemoteIdentifiedCallback(func(l *Link, id *identity.Identity) {
		identifiedRemote = id
		identWg.Done()
	})

	if err := linkB.Identify(idB); err != nil {
		t.Fatalf("Identify failed: %v", err)
	}

	identDone := make(chan struct{})
	go func() {
		identWg.Wait()
		close(identDone)
	}()

	select {
	case <-identDone:
		if !bytes.Equal(identifiedRemote.GetPublicKey(), idB.GetPublicKey()) {
			t.Error("Identified public key mismatch")
		} else {
			t.Log("Node B identified successfully to Node A")
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting for identification")
	}

	// Test path discovery for unknown destination
	t.Log("Testing path discovery for unknown destination...")
	// Create another destination on Node A that B doesn't know about yet
	destA2, _ := destination.New(idA, destination.In, destination.Single, "anotherapp", trA)
	destA2.AcceptsLinks(true)

	// Node B shouldn't have a path yet
	if trB.HasPath(destA2.GetHash()) {
		t.Error("Node B should NOT have a path to destA2 yet")
	}

	if err := trB.RequestPath(destA2.GetHash(), "pipeB", nil, false); err != nil {
		t.Errorf("Path request failed: %v", err)
	}

	// Wait for discovery
	discovered := false
	for range 10 {
		if trB.HasPath(destA2.GetHash()) {
			discovered = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if !discovered {
		t.Error("Node B failed to discover path to destA2 after path request")
	} else {
		t.Log("Node B discovered path to destA2 successfully")
	}
}

func TestLinkRequestResponseInterop(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	// Create Nodes
	cfgA := &common.ReticulumConfig{}
	trA := transport.NewTransport(cfgA)
	defer trA.Close()
	idA, _ := identity.New()

	cfgB := &common.ReticulumConfig{}
	trB := transport.NewTransport(cfgB)
	defer trB.Close()
	idB, _ := identity.New()
	_ = idB // Use idB to avoid unused error

	// Connect
	pipeA := NewPipeInterface("pipeA")
	pipeB := NewPipeInterface("pipeB")
	pipeA.peer = pipeB
	pipeB.peer = pipeA
	pipeA.tr = trA
	pipeB.tr = trB
	_ = trA.RegisterInterface("pipeA", pipeA)
	_ = trB.RegisterInterface("pipeB", pipeB)

	// Setup Destination on A
	destA, _ := destination.New(idA, destination.In, destination.Single, "reqapp", trA)
	destA.AcceptsLinks(true)

	// Register multiple handlers to test routing
	destA.RegisterRequestHandler("test/1", func(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) []byte {
		return []byte("resp 1")
	}, destination.AllowAll, nil)

	destA.RegisterRequestHandler("test/2", func(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) []byte {
		return append([]byte("resp 2: "), data...)
	}, destination.AllowAll, nil)

	// Node A announces
	_ = destA.Announce(false, nil, nil)
	time.Sleep(100 * time.Millisecond)

	// Node B establishes link
	var wg sync.WaitGroup
	wg.Add(1)
	linkB := NewLink(destA, trB, pipeB, func(l *Link) {
		wg.Done()
	}, nil)
	_ = linkB.Establish()

	select {
	case <-time.After(2 * time.Second):
		t.Fatal("Link timeout")
	case <-func() chan struct{} {
		ch := make(chan struct{})
		go func() { wg.Wait(); close(ch) }()
		return ch
	}():
	}

	// Test Request 1
	receipt1, err := linkB.Request("test/1", nil, 2*time.Second)
	if err != nil {
		t.Fatalf("Request 1 failed: %v", err)
	}

	done1 := make(chan struct{})
	receipt1.SetResponseCallback(func(r *RequestReceipt) {
		if !bytes.Equal(r.GetResponse(), []byte("resp 1")) {
			t.Errorf("Response 1 mismatch: %q", r.GetResponse())
		}
		close(done1)
	})

	// Test Request 2
	payload2 := []byte("hello")
	receipt2, err := linkB.Request("test/2", payload2, 2*time.Second)
	if err != nil {
		t.Fatalf("Request 2 failed: %v", err)
	}

	done2 := make(chan struct{})
	receipt2.SetResponseCallback(func(r *RequestReceipt) {
		if !bytes.Equal(r.GetResponse(), []byte("resp 2: hello")) {
			t.Errorf("Response 2 mismatch: %q", r.GetResponse())
		}
		close(done2)
	})

	// Wait for both
	select {
	case <-done1:
		t.Log("Request 1 success")
	case <-time.After(2 * time.Second):
		t.Error("Request 1 timeout")
	}

	select {
	case <-done2:
		t.Log("Request 2 success")
	case <-time.After(2 * time.Second):
		t.Error("Request 2 timeout")
	}
}
