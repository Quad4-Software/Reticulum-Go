// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

var zeroTime time.Time

func newIdentity(t *testing.T) *identity.Identity {
	t.Helper()
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	return id
}

func TestNewDestination_NilIdentityNonPlain(t *testing.T) {
	if _, err := New(nil, In, Single, "app", &mockTransport{}); err == nil {
		t.Fatal("expected error for nil identity with non-Plain destination")
	}
}

func TestFromHash_NilIdentityNonPlain(t *testing.T) {
	if _, err := FromHash(make([]byte, 16), nil, Single, &mockTransport{}); err == nil {
		t.Fatal("expected error for nil identity with non-Plain FromHash")
	}
}

func TestFromHash_PlainNilIdentityOK(t *testing.T) {
	hash := make([]byte, 16)
	hash[0] = 0x77
	d, err := FromHash(hash, nil, Plain, &mockTransport{})
	if err != nil {
		t.Fatalf("FromHash Plain: %v", err)
	}
	if !bytes.Equal(d.GetHash(), hash) {
		t.Fatal("hash mismatch")
	}
	if d.GetType() != Plain {
		t.Errorf("GetType: got %d, want Plain", d.GetType())
	}
}

func TestAcceptsLinksRegistersWithTransport(t *testing.T) {
	tr := &mockTransport{}
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", tr)

	registered := make(map[string]bool)
	tr2 := &registeringTransport{registered: registered}
	d.transport = tr2

	d.AcceptsLinks(true)
	if !registered[string(d.GetHash())] {
		t.Error("AcceptsLinks(true) should register destination with transport")
	}
	d.AcceptsLinks(false)
	// Toggling false must not panic and should leave the flag cleared.
	if d.acceptsLinks {
		t.Error("AcceptsLinks(false) should clear flag")
	}
}

type registeringTransport struct {
	registered map[string]bool
}

func (r *registeringTransport) GetConfig() *common.ReticulumConfig                { return nil }
func (r *registeringTransport) GetInterfaces() map[string]common.NetworkInterface { return nil }
func (r *registeringTransport) RegisterDestination(hash []byte, dest any) {
	r.registered[string(hash)] = true
}

func TestLinkAndProofCallbacks(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})

	if d.GetLinkCallback() != nil {
		t.Fatal("expected nil link callback initially")
	}
	cb := func(link any) {}
	d.SetLinkEstablishedCallback(cb)
	if d.GetLinkCallback() == nil {
		t.Fatal("link callback not set")
	}

	d.SetProofRequestedCallback(func(hash []byte, appData []byte) {})
	d.SetProofStrategy(ProveApp)
	if d.proofStrategy != ProveApp {
		t.Errorf("proof strategy: got %d, want %d", d.proofStrategy, ProveApp)
	}
}

func TestReceiveNoCallback(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})
	// No packet callback set: must not panic.
	d.Receive(&packet.Packet{PacketType: packet.PacketTypeData, Data: []byte("x")}, nil)
}

func TestReceiveDataPacketInvokesCallback(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Plain, "app", &mockTransport{})

	got := make(chan []byte, 1)
	d.SetPacketCallback(func(data []byte, iface common.NetworkInterface) {
		got <- data
	})
	d.Receive(&packet.Packet{PacketType: packet.PacketTypeData, Data: []byte("hello")}, nil)
	select {
	case b := <-got:
		if string(b) != "hello" {
			t.Errorf("callback received %q", b)
		}
	default:
		t.Fatal("callback not invoked")
	}
}

func TestReceiveLinkRequestWithoutHandler(t *testing.T) {
	RegisterIncomingLinkHandler(nil)
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})
	called := false
	d.SetPacketCallback(func(data []byte, iface common.NetworkInterface) {
		called = true
	})
	// No incoming link handler registered yet: error is logged, callback not
	// invoked for link requests.
	d.Receive(&packet.Packet{PacketType: packet.PacketTypeLinkReq}, nil)
	if called {
		t.Fatal("data callback should not fire for link request without handler")
	}
}

func TestHandleIncomingLinkRequestInvalidPacketType(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})
	if err := d.HandleIncomingLinkRequest("not a packet", nil, nil); err == nil {
		t.Fatal("expected error for invalid packet type")
	}
}

func TestHandleIncomingLinkRequestWithHandler(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})

	handlerCalled := false
	RegisterIncomingLinkHandler(func(pkt *packet.Packet, dest *Destination, transport any, iface common.NetworkInterface) (any, error) {
		handlerCalled = true
		return struct{}{}, nil
	})
	defer RegisterIncomingLinkHandler(nil)

	if err := d.HandleIncomingLinkRequest(&packet.Packet{PacketType: packet.PacketTypeLinkReq}, nil, nil); err != nil {
		t.Fatalf("HandleIncomingLinkRequest: %v", err)
	}
	if !handlerCalled {
		t.Fatal("handler not invoked")
	}

	// After clearing the global handler, calls must fail cleanly.
	RegisterIncomingLinkHandler(nil)
	if err := d.HandleIncomingLinkRequest(&packet.Packet{PacketType: packet.PacketTypeLinkReq}, nil, nil); err == nil {
		t.Fatal("expected error after handler cleared")
	}
}

func TestRatchetConfigSetters(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In|Out, Single, "app", &mockTransport{})

	if d.SetRetainedRatchets(0) {
		t.Error("SetRetainedRatchets(0) should reject non-positive count")
	}
	if !d.SetRetainedRatchets(8) {
		t.Fatal("SetRetainedRatchets(8) should succeed")
	}
	if d.ratchetCount != 8 {
		t.Errorf("ratchetCount: got %d, want 8", d.ratchetCount)
	}

	if d.SetRatchetInterval(0) {
		t.Error("SetRatchetInterval(0) should reject non-positive interval")
	}
	if !d.SetRatchetInterval(60) {
		t.Fatal("SetRatchetInterval(60) should succeed")
	}
	if d.ratchetInterval != 60 {
		t.Errorf("ratchetInterval: got %d, want 60", d.ratchetInterval)
	}

	d.EnforceRatchets()
	if !d.enforceRatchets {
		t.Error("EnforceRatchets should set flag")
	}
}

func TestRotateRatchetsNotEnabled(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In|Out, Single, "app", &mockTransport{})
	if err := d.RotateRatchets(); err == nil {
		t.Fatal("expected error rotating ratchets when not enabled")
	}
}

func TestRotateRatchetsIntervalNotReached(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In|Out, Single, "app", &mockTransport{})
	d.EnableRatchets(filepath.Join(t.TempDir(), "ratchets"))

	if err := d.RotateRatchets(); err != nil {
		t.Fatalf("first RotateRatchets: %v", err)
	}
	if err := d.RotateRatchets(); err != nil {
		t.Fatalf("second RotateRatchets: %v", err)
	}
	if got := len(d.GetRatchets()); got != 1 {
		t.Errorf("interval not reached: expected 1 ratchet, got %d", got)
	}
}

func TestCleanRatchetsTrims(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In|Out, Single, "app", &mockTransport{})
	d.EnableRatchets(filepath.Join(t.TempDir(), "ratchets"))
	d.SetRetainedRatchets(2)

	for range 5 {
		if err := d.RotateRatchets(); err != nil {
			t.Fatalf("RotateRatchets: %v", err)
		}
		// Force the rotation interval to elapse so the next call rotates again.
		d.latestRatchetTime = d.latestRatchetTime.Add(-time.Duration(d.ratchetInterval) * time.Second)
	}
	if got := len(d.GetRatchets()); got != 2 {
		t.Errorf("expected ratchet count trimmed to 2, got %d", got)
	}
}

func TestGetRatchetsDisabled(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In|Out, Single, "app", &mockTransport{})
	if got := d.GetRatchets(); got != nil {
		t.Errorf("expected nil ratchets when disabled, got %v", got)
	}
}

func TestDefaultAppDataClear(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In|Out, Single, "app", &mockTransport{})
	d.SetDefaultAppData([]byte("payload"))
	if !bytes.Equal(d.defaultAppData, []byte("payload")) {
		t.Error("SetDefaultAppData did not store payload")
	}
	d.ClearDefaultAppData()
	if d.defaultAppData != nil {
		t.Error("ClearDefaultAppData should nil out app data")
	}
}

func TestRegisterRequestHandlerAnyErrors(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})
	gen := func(p string, data []byte, rid []byte, lid []byte, ri *identity.Identity, ra int64) any {
		return []byte("x")
	}
	if err := d.RegisterRequestHandlerAny("", gen, AllowAll, nil); err == nil {
		t.Fatal("expected error for empty path")
	}
	if err := d.RegisterRequestHandlerAny("p", gen, 0x99, nil); err == nil {
		t.Fatal("expected error for invalid allow mode")
	}
	if err := d.RegisterRequestHandlerAny("p", gen, AllowList, nil); err == nil {
		t.Fatal("expected error for AllowList with empty list")
	}
	if err := d.RegisterRequestHandlerAny("p", gen, AllowList, [][]byte{make([]byte, 16)}); err != nil {
		t.Fatalf("valid AllowList registration: %v", err)
	}
}

func TestDeregisterUnknownHandler(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})
	if d.DeregisterRequestHandler("nope") {
		t.Error("DeregisterRequestHandler should return false for unknown path")
	}
}

func TestHandleRequestNotFound(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})
	got := d.HandleRequest("missing", nil, nil, nil, nil, 0)
	if !bytes.Contains(got, []byte("Not Found")) {
		t.Errorf("expected Not Found response, got %q", got)
	}
}

func TestHandleRequestNilResult(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})
	_ = d.RegisterRequestHandlerAny("nil", func(p string, _ []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) any {
		return nil
	}, AllowAll, nil)
	got := d.HandleRequest("nil", nil, nil, nil, nil, 0)
	if !bytes.Contains(got, []byte("Not Found")) {
		t.Errorf("nil result should yield Not Found, got %q", got)
	}
}

func TestHandleRequestEncodesNonByteResult(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})
	_ = d.RegisterRequestHandlerAny("map", func(p string, _ []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) any {
		return map[string]any{"k": "v"}
	}, AllowAll, nil)
	got := d.HandleRequest("map", nil, nil, nil, nil, 0)
	if len(got) == 0 || bytes.Contains(got, []byte("Not Found")) {
		t.Fatalf("expected msgpack-encoded result, got %q", got)
	}
}

func TestGetRequestHandlerAllowAll(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})
	_ = d.RegisterRequestHandlerAny("/path", func(p string, _ []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) any {
		return []byte("allowed")
	}, AllowAll, nil)

	pathHash := identity.TruncatedHash([]byte("/path"))
	h := d.GetRequestHandler(pathHash)
	if h == nil {
		t.Fatal("expected handler for AllowAll")
	}
	out := h(pathHash, nil, nil, nil, nil, zeroTime)
	if !bytes.Equal(out.([]byte), []byte("allowed")) {
		t.Errorf("handler output: got %v", out)
	}
}

func TestGetRequestHandlerAllowListMatchAndMiss(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})

	allowed := newIdentity(t)
	other := newIdentity(t)
	_ = d.RegisterRequestHandlerAny("/secret", func(p string, _ []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) any {
		return []byte("secret")
	}, AllowList, [][]byte{allowed.Hash()})

	pathHash := identity.TruncatedHash([]byte("/secret"))
	h := d.GetRequestHandler(pathHash)
	if h == nil {
		t.Fatal("expected handler")
	}
	if out := h(pathHash, nil, nil, nil, allowed, zeroTime); out != nil {
		if !bytes.Equal(out.([]byte), []byte("secret")) {
			t.Errorf("allowed identity: got %v", out)
		}
	} else {
		t.Fatal("allowed identity should yield response")
	}
	if out := h(pathHash, nil, nil, nil, other, zeroTime); out != nil {
		t.Errorf("non-allowed identity should be rejected, got %v", out)
	}
	// AllowList with nil remote identity: not allowed.
	if out := h(pathHash, nil, nil, nil, nil, zeroTime); out != nil {
		t.Errorf("nil remote identity should be rejected, got %v", out)
	}
}

func TestGetRequestHandlerAllowNoneRejects(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})
	_ = d.RegisterRequestHandlerAny("/closed", func(p string, _ []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) any {
		return []byte("x")
	}, AllowNone, nil)
	pathHash := identity.TruncatedHash([]byte("/closed"))
	h := d.GetRequestHandler(pathHash)
	if h == nil {
		t.Fatal("expected handler")
	}
	if out := h(pathHash, nil, nil, nil, id, zeroTime); out != nil {
		t.Errorf("AllowNone should reject, got %v", out)
	}
}

func TestGetRequestHandlerUnknownPath(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})
	if h := d.GetRequestHandler([]byte("does-not-match")); h != nil {
		t.Fatal("expected nil handler for unknown path hash")
	}
}

func TestEncryptUnsupportedType(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In|Out, Single, "app", &mockTransport{})
	d.destType = 0xFE // unsupported
	if _, err := d.Encrypt([]byte("x")); err == nil {
		t.Fatal("expected error for unsupported destination type")
	}
}

func TestDecryptNilIdentityNonPlain(t *testing.T) {
	d, _ := New(nil, In|Out, Plain, "app", &mockTransport{})
	d.destType = Single
	if _, err := d.Decrypt([]byte("x")); err == nil {
		t.Fatal("expected error decrypting non-Plain destination with nil identity")
	}
}

func TestSignAndGetPublicKeyNilIdentity(t *testing.T) {
	d, _ := New(nil, In|Out, Plain, "app", &mockTransport{})
	if _, err := d.Sign([]byte("x")); err == nil {
		t.Fatal("expected error signing with nil identity")
	}
	if d.GetPublicKey() != nil {
		t.Error("GetPublicKey should return nil for nil identity")
	}
}

func TestSignAndGetPublicKeyAndIdentity(t *testing.T) {
	id, _ := identity.New()
	d, _ := New(id, In|Out, Single, "app", &mockTransport{})
	sig, err := d.Sign([]byte("msg"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(sig) == 0 {
		t.Fatal("empty signature")
	}
	if !bytes.Equal(d.GetPublicKey(), id.GetPublicKey()) {
		t.Error("GetPublicKey mismatch")
	}
	if d.GetIdentity() != id {
		t.Error("GetIdentity mismatch")
	}
}
