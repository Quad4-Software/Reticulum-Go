// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
)

// TestRNodeQueueCap bounds the transmit queue while flow control holds the
// radio busy; the packet stream is remote-influenced, so the queue cannot
// grow without limit.
func TestRNodeQueueCap(t *testing.T) {
	r := &RNodeInterface{
		BaseInterface:  NewBaseInterface("rnode-cap", common.IFTypeRNode, true),
		interfaceReady: false,
	}
	r.Online = true
	for i := 0; i < rnodeMaxQueuedPackets*2; i++ {
		if err := r.ProcessOutgoing([]byte{0x01}); err != nil {
			t.Fatalf("ProcessOutgoing %d: %v", i, err)
		}
	}
	r.queueMu.Lock()
	n := len(r.packetQueue)
	r.queueMu.Unlock()
	if n != rnodeMaxQueuedPackets {
		t.Fatalf("packetQueue len = %d, want cap %d", n, rnodeMaxQueuedPackets)
	}
}

func TestRNodeSubInterfaceQueueCap(t *testing.T) {
	s := &RNodeSubInterface{
		BaseInterface:  NewBaseInterface("sub0", common.IFTypeRNodeMulti, true),
		parent:         &RNodeMultiInterface{},
		interfaceReady: false,
	}
	s.Online = true
	for i := 0; i < rnodeMaxQueuedPackets*2; i++ {
		if err := s.ProcessOutgoing([]byte{0x01}); err != nil {
			t.Fatalf("ProcessOutgoing %d: %v", i, err)
		}
	}
	s.stateMu.Lock()
	n := len(s.packetQueue)
	s.stateMu.Unlock()
	if n != rnodeMaxQueuedPackets {
		t.Fatalf("packetQueue len = %d, want cap %d", n, rnodeMaxQueuedPackets)
	}
}
