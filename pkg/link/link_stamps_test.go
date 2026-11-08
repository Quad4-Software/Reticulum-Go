// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"context"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/stamps"
)

func TestValidateLinkRequestStampEnforced(t *testing.T) {
	destHash := make([]byte, 16)
	sigPub := make([]byte, 32)
	for i := range sigPub {
		sigPub[i] = byte(i)
	}
	material := append(append([]byte{}, destHash...), sigPub...)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	priv, pub, _, err := stamps.GenerateStampedKey(ctx, material, 8)
	if err != nil {
		t.Fatal(err)
	}
	_ = priv

	l := &Link{transport: nil}
	// nil transport → defaults; LREnforced false → never rejects.
	v, err := l.validateLinkRequestStamp(destHash, 8, pub[:], sigPub)
	if err != nil || v < 8 {
		t.Fatalf("unenforced: v=%v err=%v", v, err)
	}
}

func TestStampThenEstablishTimeoutFailsLink(t *testing.T) {
	// Very high cost + tiny timeout must fail with STAMP_FAILURE.
	l := &Link{
		linkDone:        make(chan struct{}),
		pendingRequests: make([]*RequestReceipt, 0),
		closedCallback:  func(*Link) {},
		requestTime:     time.Now(),
		initiator:       true,
	}
	l.status.Store(int32(StatusPending))
	// simulate impossible generation directly at the failure seam.
	l.failStampedEstablish(make([]byte, 16), context.DeadlineExceeded)
	if l.status.Load() != int32(StatusClosed) {
		t.Fatal("link not closed")
	}
	if l.teardownReason != TeardownStampFailure {
		t.Fatalf("teardown reason %#x want %#x", l.teardownReason, TeardownStampFailure)
	}
}
