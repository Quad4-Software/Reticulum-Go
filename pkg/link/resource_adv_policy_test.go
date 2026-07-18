// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/resource"
)

func packTestAdvertisement(t *testing.T, adv *resource.ResourceAdvertisement) []byte {
	t.Helper()
	packed, err := adv.Pack(0, 384)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	return packed
}

func TestProcessResourceAdvertisement_IgnoresRequestWithoutHandlers(t *testing.T) {
	l := &Link{}
	l.mdu = 384
	l.status.Store(int32(StatusActive))
	l.resourceStrategy = AcceptAll

	adv := &resource.ResourceAdvertisement{
		Parts:        4,
		TransferSize: 4 * 384,
		DataSize:     1024,
		RandomHash:   bytes.Repeat([]byte{0x21}, resource.RandomHashSize),
		Hash:         bytes.Repeat([]byte{0x22}, 32),
		Hashmap:      makeFakeHashmap(1),
		RequestID:    bytes.Repeat([]byte{0x23}, 16),
		IsRequest:    true,
		Flags:        resource.AdvFlagIsRequest,
	}

	if err := l.processResourceAdvertisement(packTestAdvertisement(t, adv)); err != nil {
		t.Fatalf("expected silent ignore, got: %v", err)
	}
	l.incomingMu.Lock()
	rx := l.incomingRx
	l.incomingMu.Unlock()
	if rx != nil {
		t.Fatal("request advertisement accepted without handlers")
	}
	if l.GetStatus() != StatusActive {
		t.Fatalf("status = %d, want Active (no teardown on ignore)", l.GetStatus())
	}
}

func TestProcessResourceAdvertisement_AcceptsRequestWithHandlers(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	dest, err := destination.New(id, destination.Out, destination.Single, "advtest", nil, "svc")
	if err != nil {
		t.Fatalf("destination.New: %v", err)
	}
	if err := dest.RegisterRequestHandler("echo", func(string, []byte, []byte, []byte, *identity.Identity, int64) []byte {
		return []byte("ok")
	}, destination.AllowAll, nil); err != nil {
		t.Fatalf("RegisterRequestHandler: %v", err)
	}

	l := &Link{destination: dest, mdu: 384}
	l.status.Store(int32(StatusActive))
	l.resourceStrategy = AcceptAll
	defer l.resetIncomingResource()

	adv := &resource.ResourceAdvertisement{
		Parts:        4,
		TransferSize: 4 * 384,
		DataSize:     1024,
		RandomHash:   bytes.Repeat([]byte{0x31}, resource.RandomHashSize),
		Hash:         bytes.Repeat([]byte{0x32}, 32),
		Hashmap:      makeFakeHashmap(1),
		RequestID:    bytes.Repeat([]byte{0x33}, 16),
		IsRequest:    true,
		Flags:        resource.AdvFlagIsRequest,
	}

	if err := l.processResourceAdvertisement(packTestAdvertisement(t, adv)); err != nil {
		t.Fatalf("processResourceAdvertisement: %v", err)
	}
	l.incomingMu.Lock()
	rx := l.incomingRx
	l.incomingMu.Unlock()
	if rx == nil {
		t.Fatal("expected request advertisement to be accepted with handlers")
	}
}

func TestAbortInvalidResourceAdvertisement_Teardown(t *testing.T) {
	l := &Link{mdu: 384}
	l.status.Store(int32(StatusActive))
	l.resourceStrategy = AcceptAll
	closed := false
	l.closedCallback = func(*Link) { closed = true }

	adv := &resource.ResourceAdvertisement{
		Parts:        4,
		TransferSize: int64(resource.MaxEfficientSize) + 4097,
		DataSize:     1024,
		RandomHash:   bytes.Repeat([]byte{0x41}, resource.RandomHashSize),
		Hash:         bytes.Repeat([]byte{0x42}, 32),
		Hashmap:      makeFakeHashmap(1),
	}

	err := l.processResourceAdvertisement(packTestAdvertisement(t, adv))
	if err == nil || !strings.Contains(err.Error(), "MaxEfficientSize") {
		t.Fatalf("expected MaxEfficientSize rejection, got: %v", err)
	}
	if got := l.abortInvalidResourceAdvertisement(err); got != err {
		t.Fatalf("abort returned %v, want %v", got, err)
	}
	if l.GetStatus() != StatusClosed {
		t.Fatalf("status = %d, want Closed after invalid advertisement", l.GetStatus())
	}
	if !closed {
		t.Fatal("expected closed callback after teardown")
	}
}

func TestProcessResourceAdvertisement_MalformedUnpack(t *testing.T) {
	l := &Link{mdu: 384}
	l.status.Store(int32(StatusActive))

	err := l.processResourceAdvertisement([]byte{0xff, 0x00, 0x01})
	if err == nil {
		t.Fatal("expected unpack error")
	}
	_ = l.abortInvalidResourceAdvertisement(err)
	if l.GetStatus() != StatusClosed {
		t.Fatalf("status = %d, want Closed", l.GetStatus())
	}
}
