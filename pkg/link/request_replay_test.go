// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package link

import (
	"sync/atomic"
	"testing"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

func packRequest(t *testing.T, requestedAt any, pathHash []byte, payload []byte) []byte {
	t.Helper()
	plaintext, err := msgpack.Marshal([]any{requestedAt, pathHash, payload})
	if err != nil {
		t.Fatalf("msgpack.Marshal: %v", err)
	}
	return plaintext
}

func registerReplayHandler(t *testing.T, l *Link, path string, called *atomic.Bool) []byte {
	t.Helper()
	pathHash := identity.TruncatedHash([]byte(path))
	err := l.destination.RegisterRequestHandler(path, func(_ string, _ []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) []byte {
		if called != nil {
			called.Store(true)
		}
		return nil
	}, destination.AllowAll, nil)
	if err != nil {
		t.Fatalf("RegisterRequestHandler: %v", err)
	}
	return pathHash
}

func TestHandleRequestRejectsStaleTimestamp(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	_, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	var called atomic.Bool
	pathHash := registerReplayHandler(t, respLink, "replay/stale", &called)

	staleAt := time.Now().Add(-time.Duration(RequestTimestampMaxSkewPast+1) * time.Second).Unix()
	plaintext := packRequest(t, staleAt, pathHash, []byte("payload"))
	pkt := &packet.Packet{Data: plaintext}
	if err := pkt.Pack(); err != nil {
		t.Fatalf("pkt.Pack: %v", err)
	}

	if err := respLink.handleRequest(plaintext, pkt); err != nil {
		t.Fatalf("handleRequest: %v", err)
	}
	if called.Load() {
		t.Fatal("handler should not run for stale requested_at")
	}
}

func TestHandleRequestRejectsFutureTimestamp(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	_, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	var called atomic.Bool
	pathHash := registerReplayHandler(t, respLink, "replay/future", &called)

	futureAt := time.Now().Add(time.Duration(RequestTimestampMaxSkewFuture+1) * time.Second).Unix()
	plaintext := packRequest(t, futureAt, pathHash, []byte("payload"))
	pkt := &packet.Packet{Data: plaintext}
	if err := pkt.Pack(); err != nil {
		t.Fatalf("pkt.Pack: %v", err)
	}

	if err := respLink.handleRequest(plaintext, pkt); err != nil {
		t.Fatalf("handleRequest: %v", err)
	}
	if called.Load() {
		t.Fatal("handler should not run for future requested_at")
	}
}

func TestHandleRequestAcceptsCurrentTimestamp(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	_, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	var called atomic.Bool
	pathHash := registerReplayHandler(t, respLink, "replay/current", &called)

	now := float64(time.Now().Unix())
	plaintext := packRequest(t, now, pathHash, []byte("payload"))
	pkt := &packet.Packet{Data: plaintext}
	if err := pkt.Pack(); err != nil {
		t.Fatalf("pkt.Pack: %v", err)
	}

	if err := respLink.handleRequest(plaintext, pkt); err != nil {
		t.Fatalf("handleRequest: %v", err)
	}
	if !called.Load() {
		t.Fatal("handler should run for current requested_at")
	}
}

func TestParseRequestedAtFloatPreservesFraction(t *testing.T) {
	got, err := parseRequestedAt(1_700_000_000.5)
	if err != nil {
		t.Fatalf("parseRequestedAt: %v", err)
	}
	want := time.Unix(1_700_000_000, 500_000_000)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRequestTimestampValidSkewWindow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	pastEdge := now.Add(-time.Duration(RequestTimestampMaxSkewPast) * time.Second)
	if !requestTimestampValid(pastEdge, now) {
		t.Fatal("timestamp at past skew edge should be accepted")
	}
	if requestTimestampValid(pastEdge.Add(-time.Second), now) {
		t.Fatal("timestamp beyond past skew edge should be rejected")
	}

	futureEdge := now.Add(time.Duration(RequestTimestampMaxSkewFuture) * time.Second)
	if !requestTimestampValid(futureEdge, now) {
		t.Fatal("timestamp at future skew edge should be accepted")
	}
	if requestTimestampValid(futureEdge.Add(time.Second), now) {
		t.Fatal("timestamp beyond future skew edge should be rejected")
	}
}

func TestRegression_RequestReplayProtectionDoesNotBreakInterop(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)
	initLink, respLink, cleanup := establishInteropLink(t)
	defer cleanup()

	respLink.destination.RegisterRequestHandler("echo", func(_ string, data []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) []byte {
		return append([]byte("echo:"), data...)
	}, destination.AllowAll, nil)

	receipt, err := initLink.Request("echo", []byte("ping"), 3*time.Second)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	respCh := make(chan []byte, 1)
	receipt.SetResponseCallback(func(r *RequestReceipt) {
		respCh <- append([]byte(nil), r.GetResponse()...)
	})

	select {
	case got := <-respCh:
		if string(got) != "echo:ping" {
			t.Fatalf("response = %q, want echo:ping", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("response timeout")
	}
}
