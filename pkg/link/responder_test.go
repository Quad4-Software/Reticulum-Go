// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package link

import (
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

func TestResponderCallbackTiming(t *testing.T) {
	// Create Node A (Server/Responder)
	cfgA := &common.ReticulumConfig{}
	trA := transport.NewTransport(cfgA)
	defer trA.Close()
	idA, _ := identity.New()

	// Create Node B (Client/Initiator)
	cfgB := &common.ReticulumConfig{}
	trB := transport.NewTransport(cfgB)
	defer trB.Close()
	idB, _ := identity.New()
	_ = idB

	// Connect them via PipeInterface
	pipeA := NewPipeInterface("pipeA")
	pipeB := NewPipeInterface("pipeB")
	pipeA.peer = pipeB
	pipeB.peer = pipeA
	pipeA.tr = trA
	pipeB.tr = trB

	_ = trA.RegisterInterface("pipeA", pipeA)
	_ = trB.RegisterInterface("pipeB", pipeB)

	// Setup Destination on Node A
	destA, _ := destination.New(idA, destination.In, destination.Single, "testapp", trA, "service")
	destA.AcceptsLinks(true)

	var callbackTriggered atomic.Bool
	var linkStatusAtCallback atomic.Int32

	destA.SetLinkEstablishedCallback(func(l any) {
		lnk, ok := l.(*Link)
		if ok {
			linkStatusAtCallback.Store(lnk.status.Load())
			callbackTriggered.Store(true)
		}
	})

	// Node A announces itself
	_ = destA.Announce(false, nil, nil)
	time.Sleep(100 * time.Millisecond)

	// Node B establishes a link to Node A
	linkB := NewLink(destA, trB, pipeB, nil, nil)
	if err := linkB.Establish(); err != nil {
		t.Fatalf("Link establishment failed: %v", err)
	}

	// Wait for establishment with a timeout
	timeout := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timed out waiting for link establishment")
		case <-ticker.C:
			if callbackTriggered.Load() {
				status := byte(linkStatusAtCallback.Load())
				if status != StatusActive {
					t.Errorf("Callback triggered with invalid status: %d (expected %d/StatusActive)", status, StatusActive)
				}
				return
			}
		}
	}
}
