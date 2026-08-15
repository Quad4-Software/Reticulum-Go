// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/common"
)

func TestSetResponseCallbackLateFires(t *testing.T) {
	l := &Link{}
	requestID := []byte("req-id-16bytes!!")
	receipt := &RequestReceipt{
		link:      l,
		requestID: requestID,
		status:    StatusPending,
	}
	l.requestMutex.Lock()
	l.pendingRequests = append(l.pendingRequests, receipt)
	l.requestMutex.Unlock()

	plaintext, err := msgpack.Marshal([]any{requestID, []byte("pong")})
	if err != nil {
		t.Fatalf("msgpack.Marshal: %v", err)
	}
	if err := l.handleResponse(plaintext); err != nil {
		t.Fatalf("handleResponse: %v", err)
	}
	if receipt.GetStatus() != StatusActive {
		t.Fatalf("status=%v want Active", receipt.GetStatus())
	}
	if !bytes.Equal(receipt.GetResponse(), []byte("pong")) {
		t.Fatalf("response=%q", receipt.GetResponse())
	}

	done := make(chan struct{})
	receipt.SetResponseCallback(func(r *RequestReceipt) {
		if !bytes.Equal(r.GetResponse(), []byte("pong")) {
			t.Errorf("late callback response=%q", r.GetResponse())
		}
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("late response callback was not fired")
	}
}

func TestSetFailedCallbackLateFires(t *testing.T) {
	receipt := &RequestReceipt{status: StatusFailed}
	done := make(chan struct{})
	receipt.SetFailedCallback(func(*RequestReceipt) { close(done) })
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("late failed callback was not fired")
	}
}

func TestRegisterPendingRequestRejectsDuplicateAndCap(t *testing.T) {
	l := &Link{}
	pathHash := bytes.Repeat([]byte{0x11}, 16)
	first := &RequestReceipt{pathHash: pathHash}
	if err := l.registerPendingRequest(first); err != nil {
		t.Fatalf("first register: %v", err)
	}
	dup := &RequestReceipt{pathHash: pathHash}
	if err := l.registerPendingRequest(dup); !errors.Is(err, common.ErrLinkRequestDuplicate) {
		t.Fatalf("duplicate = %v, want ErrLinkRequestDuplicate", err)
	}
	l.pendingRequests = nil
	for i := range MaxPendingRequests {
		h := bytes.Repeat([]byte{byte(i + 1)}, 16)
		if err := l.registerPendingRequest(&RequestReceipt{pathHash: h}); err != nil {
			t.Fatalf("fill %d: %v", i, err)
		}
	}
	extra := &RequestReceipt{pathHash: bytes.Repeat([]byte{0xFF}, 16)}
	if err := l.registerPendingRequest(extra); !errors.Is(err, common.ErrLinkRequestBusy) {
		t.Fatalf("over cap = %v, want ErrLinkRequestBusy", err)
	}
}
