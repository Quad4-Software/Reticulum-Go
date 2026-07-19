// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"errors"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

func newFSMLink(t *testing.T) (*Link, *transport.Transport, func()) {
	t.Helper()
	cfg := &common.ReticulumConfig{}
	tr := transport.NewTransport(cfg)
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	dest, err := destination.New(id, destination.Out, destination.Single, "fsmapp", tr, "svc")
	if err != nil {
		t.Fatalf("destination.New: %v", err)
	}
	l := NewLink(dest, tr, nil, nil, nil)
	cleanup := func() { _ = tr.Close() }
	return l, tr, cleanup
}

func waitStatus(t *testing.T, l *Link, want byte, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if l.GetStatus() == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("status = %d, want %d within %v", l.GetStatus(), want, timeout)
}

func TestLinkFSM_NewLinkIsPending(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	if l.GetStatus() != StatusPending {
		t.Fatalf("new link status = %d, want Pending", l.GetStatus())
	}
}

func TestLinkFSM_ReestablishRejectsLiveStates(t *testing.T) {
	cases := []byte{StatusPending, StatusHandshake, StatusActive}
	for _, st := range cases {
		t.Run(statusName(st), func(t *testing.T) {
			l, _, cleanup := newFSMLink(t)
			defer cleanup()
			l.status.Store(int32(st))
			err := l.Reestablish()
			if err == nil {
				t.Fatal("expected Reestablish to reject live state")
			}
			if l.GetStatus() != st {
				t.Fatalf("status changed to %d after rejected Reestablish", l.GetStatus())
			}
		})
	}
}

func TestLinkFSM_ClosedIsAbsorbingUntilReestablish(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.status.Store(int32(StatusClosed))
	l.Teardown()
	if l.GetStatus() != StatusClosed {
		t.Fatalf("Teardown on Closed changed status to %d", l.GetStatus())
	}
	// Reestablish resets to Pending then Establish fails without a path.
	err := l.Reestablish()
	if err == nil {
		t.Fatal("expected Establish failure without path")
	}
	if !errors.Is(err, common.ErrLinkNoPath) {
		t.Fatalf("Reestablish error = %v, want ErrLinkNoPath", err)
	}
	if l.GetStatus() != StatusPending {
		t.Fatalf("after Reestablish status = %d, want Pending", l.GetStatus())
	}
}

func TestLinkFSM_PendingEstablishmentTimeout(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.establishmentTimeout = 25 * time.Millisecond
	l.requestTime = time.Now().Add(-time.Second)
	l.linkID = make([]byte, 16)
	l.status.Store(int32(StatusPending))
	l.startWatchdog()
	waitStatus(t, l, StatusClosed, 2*time.Second)
	l.mutex.Lock()
	reason := l.teardownReason
	l.mutex.Unlock()
	if reason != StatusFailed {
		t.Fatalf("teardownReason = %d, want Failed", reason)
	}
}

func TestLinkFSM_HandshakeEstablishmentTimeout(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.establishmentTimeout = 25 * time.Millisecond
	l.requestTime = time.Now().Add(-time.Second)
	l.linkID = make([]byte, 16)
	l.status.Store(int32(StatusHandshake))
	l.startWatchdog()
	waitStatus(t, l, StatusClosed, 2*time.Second)
	l.mutex.Lock()
	reason := l.teardownReason
	l.mutex.Unlock()
	if reason != StatusFailed {
		t.Fatalf("teardownReason = %d, want Failed", reason)
	}
}

func TestLinkFSM_ActiveToStale(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.keepalive = 15 * time.Millisecond
	l.staleTime = 30 * time.Millisecond
	l.establishedAt = time.Now().Add(-time.Hour)
	l.lastInboundNs.Store(0)
	l.lastOutboundNs.Store(0)
	l.lastDataSentNs.Store(0)
	l.linkID = make([]byte, 16)
	l.status.Store(int32(StatusActive))
	l.startWatchdog()
	// Watchdog sleeps StaleGrace seconds after marking Stale before closing.
	waitStatus(t, l, StatusStale, time.Second)
}

func TestLinkFSM_StaleToClosedViaWatchdog(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.linkID = make([]byte, 16)
	_ = setSecBuf(&l.sessionKey, make([]byte, 32))
	_ = setSecBuf(&l.hmacKey, make([]byte, 32))
	l.status.Store(int32(StatusStale))
	l.startWatchdog()
	waitStatus(t, l, StatusClosed, time.Second)
	l.mutex.Lock()
	reason := l.teardownReason
	l.mutex.Unlock()
	if reason != StatusFailed {
		t.Fatalf("teardownReason = %d, want Failed", reason)
	}
}

func TestLinkFSM_StaleToActiveOnInbound(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.linkID = make([]byte, 16)
	_ = setSecBuf(&l.sessionKey, make([]byte, 32))
	_ = setSecBuf(&l.hmacKey, make([]byte, 32))
	l.status.Store(int32(StatusStale))
	// Do not start watchdog: it would close Stale immediately.
	pkt := &packet.Packet{
		PacketType:      packet.PacketTypeData,
		Context:         packet.ContextNone,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Data:            []byte{0x00},
	}
	_ = l.HandleInbound(pkt)
	if l.GetStatus() != StatusActive {
		t.Fatalf("status = %d, want Active after inbound on Stale", l.GetStatus())
	}
}

func TestLinkFSM_TeardownActiveToClosed(t *testing.T) {
	l, _, cleanup := newFSMLink(t)
	defer cleanup()
	l.linkID = make([]byte, 16)
	_ = setSecBuf(&l.sessionKey, make([]byte, 32))
	_ = setSecBuf(&l.hmacKey, make([]byte, 32))
	l.status.Store(int32(StatusActive))
	l.Teardown()
	if l.GetStatus() != StatusClosed {
		t.Fatalf("status = %d after Teardown, want Closed", l.GetStatus())
	}
}

func TestLinkFSM_ActiveInvariantRequiresKeys(t *testing.T) {
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()
	if initLink.GetStatus() != StatusActive || respLink.GetStatus() != StatusActive {
		t.Fatalf("expected Active peers, got init=%d resp=%d", initLink.GetStatus(), respLink.GetStatus())
	}
	if bufLen(initLink.sessionKey) == 0 || bufLen(initLink.hmacKey) == 0 {
		t.Fatal("Active initiator missing session/hmac keys")
	}
	if bufLen(respLink.sessionKey) == 0 || bufLen(respLink.hmacKey) == 0 {
		t.Fatal("Active responder missing session/hmac keys")
	}
}

func statusName(st byte) string {
	switch st {
	case StatusPending:
		return "Pending"
	case StatusHandshake:
		return "Handshake"
	case StatusActive:
		return "Active"
	case StatusStale:
		return "Stale"
	case StatusClosed:
		return "Closed"
	case StatusFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}
