// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package channel

import (
	"testing"
)

// TestHandleInboundDropsFarAheadSequence pins the Python Channel._receive
// check that drops sequences more than WINDOW_MAX ahead of nextRxSequence.
// Without it a peer can pin one rxRing entry per sequence value.
func TestHandleInboundDropsFarAheadSequence(t *testing.T) {
	c := NewChannel(&mockLink{status: 1})
	defer func() { _ = c.Close() }()

	far := packInboundEnvelope(t, uint16(WindowMax)+1)
	if err := c.HandleInbound(far); err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}

	c.mutex.Lock()
	ringLen := len(c.rxRing)
	c.mutex.Unlock()
	if ringLen != 0 {
		t.Fatalf("far-ahead sequence was buffered, rxRing len = %d", ringLen)
	}
}

// TestHandleInboundBuffersInWindowSequence is the control: a sequence at the
// window edge is out of order but in window, so it stays buffered for
// reordering.
func TestHandleInboundBuffersInWindowSequence(t *testing.T) {
	c := NewChannel(&mockLink{status: 1})
	defer func() { _ = c.Close() }()

	edge := packInboundEnvelope(t, uint16(WindowMax))
	if err := c.HandleInbound(edge); err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}

	c.mutex.Lock()
	ringLen := len(c.rxRing)
	c.mutex.Unlock()
	if ringLen != 1 {
		t.Fatalf("in-window sequence not buffered, rxRing len = %d", ringLen)
	}
}

// TestHandleInboundFarAheadFloodStaysBounded sends a spread of sequences well
// past the window. The ring must stay empty for every one.
func TestHandleInboundFarAheadFloodStaysBounded(t *testing.T) {
	c := NewChannel(&mockLink{status: 1})
	defer func() { _ = c.Close() }()

	for seq := uint16(WindowMax) + 1; seq < uint16(WindowMax)+200; seq++ {
		if err := c.HandleInbound(packInboundEnvelope(t, seq)); err != nil {
			t.Fatalf("HandleInbound seq %d: %v", seq, err)
		}
	}

	c.mutex.Lock()
	ringLen := len(c.rxRing)
	c.mutex.Unlock()
	if ringLen != 0 {
		t.Fatalf("far-ahead flood left %d buffered envelopes", ringLen)
	}
}

func packInboundEnvelope(t *testing.T, seq uint16) []byte {
	t.Helper()
	raw, err := packEnvelope(1, seq, []byte("x"))
	if err != nil {
		t.Fatalf("packEnvelope: %v", err)
	}
	return raw
}
