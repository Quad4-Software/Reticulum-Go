// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/announce"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

type mockTransport struct {
	config     *common.ReticulumConfig
	interfaces map[string]common.NetworkInterface
}

func (m *mockTransport) GetConfig() *common.ReticulumConfig {
	return m.config
}

func (m *mockTransport) GetInterfaces() map[string]common.NetworkInterface {
	return m.interfaces
}

func (m *mockTransport) RegisterDestination(hash []byte, dest any) {
}

type recordingInterface struct {
	common.BaseInterface
	mu       sync.Mutex
	sent     [][]byte
	bwOK     bool
	enabled  bool
	online   bool
	detached bool
}

func newRecordingInterface(name string) *recordingInterface {
	ri := &recordingInterface{
		bwOK:    true,
		enabled: true,
		online:  true,
	}
	ri.Name = name
	ri.Online = true
	ri.Enabled = true
	return ri
}

func (r *recordingInterface) Send(data []byte, address string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	r.sent = append(r.sent, cp)
	return nil
}

func (r *recordingInterface) Sent() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]byte, len(r.sent))
	copy(out, r.sent)
	return out
}

func (r *recordingInterface) IsEnabled() bool             { return r.enabled }
func (r *recordingInterface) IsOnline() bool              { return r.online }
func (r *recordingInterface) IsDetached() bool            { return r.detached }
func (r *recordingInterface) GetBandwidthAvailable() bool { return r.bwOK }

func TestNewDestination(t *testing.T) {
	id, _ := identity.New()
	transport := &mockTransport{config: &common.ReticulumConfig{}}

	dest, err := New(id, In|Out, Single, "testapp", transport, "testaspect")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if dest == nil {
		t.Fatal("New returned nil")
	}

	if dest.ExpandName() != "testapp.testaspect" {
		t.Errorf("Expected name testapp.testaspect, got %s", dest.ExpandName())
	}

	hash := dest.GetHash()
	if len(hash) != 16 {
		t.Errorf("Expected hash length 16, got %d", len(hash))
	}
}

func TestFromHash(t *testing.T) {
	id, _ := identity.New()
	transport := &mockTransport{}
	hash := make([]byte, 16)

	dest, err := FromHash(hash, id, Single, transport)
	if err != nil {
		t.Fatalf("FromHash failed: %v", err)
	}
	if !bytes.Equal(dest.GetHash(), hash) {
		t.Error("Hashes don't match")
	}
}

func TestRequestHandlers(t *testing.T) {
	id, _ := identity.New()
	dest, _ := New(id, In, Single, "test", &mockTransport{})

	path := "test/path"
	response := []byte("hello")

	err := dest.RegisterRequestHandler(path, func(p string, d []byte, rid []byte, lid []byte, ri *identity.Identity, ra int64) []byte {
		return response
	}, AllowAll, nil)
	if err != nil {
		t.Fatalf("RegisterRequestHandler failed: %v", err)
	}

	result := dest.HandleRequest(path, nil, nil, nil, nil, 0)
	if !bytes.Equal(result, response) {
		t.Errorf("Expected response %q, got %q", response, result)
	}

	if !dest.DeregisterRequestHandler(path) {
		t.Error("DeregisterRequestHandler failed")
	}

	if dest.HasRequestHandlers() {
		t.Error("HasRequestHandlers should be false after deregister")
	}
}

func TestHasRequestHandlers(t *testing.T) {
	var nilDest *Destination
	if nilDest.HasRequestHandlers() {
		t.Error("nil destination should report no handlers")
	}

	id, _ := identity.New()
	dest, _ := New(id, In, Single, "test", &mockTransport{})
	if dest.HasRequestHandlers() {
		t.Error("new destination should have no handlers")
	}

	if err := dest.RegisterRequestHandler("p", func(string, []byte, []byte, []byte, *identity.Identity, int64) []byte {
		return nil
	}, AllowAll, nil); err != nil {
		t.Fatalf("RegisterRequestHandler: %v", err)
	}
	if !dest.HasRequestHandlers() {
		t.Error("expected HasRequestHandlers true after register")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	id, _ := identity.New()
	dest, _ := New(id, In|Out, Single, "test", &mockTransport{})

	plaintext := []byte("hello world")
	ciphertext, err := dest.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := dest.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypted data doesn't match: %q vs %q", decrypted, plaintext)
	}
}

func TestRatchets(t *testing.T) {
	tmpDir := t.TempDir()
	ratchetPath := filepath.Join(tmpDir, "ratchets")

	id, _ := identity.New()
	dest, _ := New(id, In|Out, Single, "test", &mockTransport{})

	if !dest.EnableRatchets(ratchetPath) {
		t.Fatal("EnableRatchets failed")
	}

	err := dest.RotateRatchets()
	if err != nil {
		t.Fatalf("RotateRatchets failed: %v", err)
	}

	ratchets := dest.GetRatchets()
	if len(ratchets) != 1 {
		t.Errorf("Expected 1 ratchet, got %d", len(ratchets))
	}
}

func TestPlainDestination(t *testing.T) {
	id, _ := identity.New()
	dest, _ := New(id, In|Out, Plain, "test", &mockTransport{})

	plaintext := []byte("plain text")
	ciphertext, _ := dest.Encrypt(plaintext)
	if !bytes.Equal(plaintext, ciphertext) {
		t.Error("Plain destination should not encrypt")
	}

	decrypted, _ := dest.Decrypt(ciphertext)
	if !bytes.Equal(plaintext, decrypted) {
		t.Error("Plain destination should not decrypt")
	}
}

func TestAnnounceFanoutAndFreshness(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}

	ifaces := map[string]common.NetworkInterface{
		"if-a": newRecordingInterface("if-a"),
		"if-b": newRecordingInterface("if-b"),
		"if-c": newRecordingInterface("if-c"),
	}
	tr := &mockTransport{config: &common.ReticulumConfig{}, interfaces: ifaces}

	dest, err := New(id, In|Out, Single, "testapp", tr, "testaspect")
	if err != nil {
		t.Fatalf("New destination: %v", err)
	}
	dest.SetDefaultAppData([]byte("appdata"))

	const announces = 4
	before := time.Now().Unix()
	for i := range announces {
		if err := dest.Announce(false, nil, nil); err != nil {
			t.Fatalf("Announce iter %d: %v", i, err)
		}
	}
	after := time.Now().Unix()

	seenRandomHashes := map[string]int{}
	for name, iface := range ifaces {
		ri := iface.(*recordingInterface)
		sent := ri.Sent()
		if len(sent) != announces {
			t.Fatalf("interface %s received %d announces, want %d", name, len(sent), announces)
		}

		rhStart := announce.HeaderType1Offset + announce.AnnounceRandomOffset
		for i, pkt := range sent {
			if len(pkt) < rhStart+announce.RandomHashSize {
				t.Fatalf("%s pkt %d too short: %d bytes", name, i, len(pkt))
			}
			rh := pkt[rhStart : rhStart+announce.RandomHashSize]
			seenRandomHashes[string(rh)]++

			tsBytes := rh[5:]
			padded := make([]byte, 8)
			copy(padded[8-len(tsBytes):], tsBytes)
			ts := int64(binary.BigEndian.Uint64(padded)) // #nosec G115
			if ts < before-1 || ts > after+1 {
				t.Fatalf("%s pkt %d: decoded ts %d outside window [%d,%d] (raw=%x)", name, i, ts, before, after, tsBytes)
			}
		}
	}

	for rh, n := range seenRandomHashes {
		if n != len(ifaces) {
			t.Fatalf("random hash %x appeared on %d/%d interfaces; expected exactly one announce per interface", []byte(rh), n, len(ifaces))
		}
	}
	if got, want := len(seenRandomHashes), announces; got != want {
		t.Fatalf("got %d distinct random hashes, want %d (one per Announce call)", got, want)
	}
}

// TestAnnounceSkipsOfflineOrDisabledInterfaces ensures a periodic
// announcer doesn't silently disappear from the network because
// one interface has flapped offline. It must still emit on every

// other healthy interface.
func TestAnnounceSkipsOfflineOrDisabledInterfaces(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}

	healthy := newRecordingInterface("healthy")
	offline := newRecordingInterface("offline")
	offline.online = false
	disabled := newRecordingInterface("disabled")
	disabled.enabled = false

	tr := &mockTransport{
		config: &common.ReticulumConfig{},
		interfaces: map[string]common.NetworkInterface{
			"healthy":  healthy,
			"offline":  offline,
			"disabled": disabled,
		},
	}

	dest, err := New(id, In|Out, Single, "testapp", tr, "testaspect")
	if err != nil {
		t.Fatalf("New destination: %v", err)
	}

	for i := range 3 {
		if err := dest.Announce(false, nil, nil); err != nil {
			t.Fatalf("Announce iter %d: %v", i, err)
		}
	}

	if got := len(healthy.Sent()); got != 3 {
		t.Fatalf("healthy interface got %d announces, want 3", got)
	}
	if got := len(offline.Sent()); got != 0 {
		t.Fatalf("offline interface got %d announces, want 0", got)
	}
	if got := len(disabled.Sent()); got != 0 {
		t.Fatalf("disabled interface got %d announces, want 0", got)
	}
}

func TestAnnounceSkipsReceiveOnlyInterfaces(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}

	tx := newRecordingInterface("tx")
	ro := newRecordingInterface("ro")
	ro.SetOutgoingAllowed(false)

	tr := &mockTransport{
		config: &common.ReticulumConfig{},
		interfaces: map[string]common.NetworkInterface{
			"tx": tx,
			"ro": ro,
		},
	}

	dest, err := New(id, In|Out, Single, "testapp", tr, "testaspect")
	if err != nil {
		t.Fatalf("New destination: %v", err)
	}
	if err := dest.Announce(false, nil, nil); err != nil {
		t.Fatalf("Announce: %v", err)
	}
	if got := len(tx.Sent()); got != 1 {
		t.Fatalf("tx got %d announces, want 1", got)
	}
	if got := len(ro.Sent()); got != 0 {
		t.Fatalf("receive-only got %d announces, want 0", got)
	}
	if err := dest.Announce(false, nil, ro); err == nil {
		t.Fatal("Announce attached receive-only should fail with 0 writable interfaces")
	} else if !errors.Is(err, common.ErrDestAnnounceNoWritable) {
		t.Fatalf("attached receive-only: got %v, want ErrDestAnnounceNoWritable", err)
	}
	if got := len(ro.Sent()); got != 0 {
		t.Fatalf("attached receive-only got %d announces, want 0", got)
	}
}

// TestAnnounceOnlyReceiveOnlyReturnsNoWritable locks the directory outgoing
// interop contract: online outgoing=no interfaces must not TX and must
// surface ErrDestAnnounceNoWritable (not a silent success).
func TestAnnounceOnlyReceiveOnlyReturnsNoWritable(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	ro := newRecordingInterface("ro")
	ro.SetOutgoingAllowed(false)
	tr := &mockTransport{
		config: &common.ReticulumConfig{},
		interfaces: map[string]common.NetworkInterface{
			"ro": ro,
		},
	}
	dest, err := New(id, In|Out, Single, "live_outgoing", tr, "probe")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = dest.Announce(false, nil, nil)
	if !errors.Is(err, common.ErrDestAnnounceNoWritable) {
		t.Fatalf("got %v, want ErrDestAnnounceNoWritable", err)
	}
	if got := len(ro.Sent()); got != 0 {
		t.Fatalf("receive-only leaked %d announces", got)
	}
}

func TestPlainDestinationHash(t *testing.T) {
	// A Plain destination with no identity should have a hash based only on its name
	transport := &mockTransport{}
	dest, err := New(nil, In|Out, Plain, "testapp", transport, "testaspect")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	hash := dest.GetHash()
	if len(hash) != 16 {
		t.Fatalf("Expected hash length 16, got %d", len(hash))
	}

	// Calculate manually: SHA256(SHA256("testapp.testaspect")[:10])[:16]
	name := "testapp.testaspect"
	nameHashFull := sha256.Sum256([]byte(name))
	nameHash10 := nameHashFull[:10]
	finalHashFull := sha256.Sum256(nameHash10)
	expectedHash := finalHashFull[:16]

	if !bytes.Equal(hash, expectedHash) {
		t.Errorf("Expected hash %x, got %x", expectedHash, hash)
	}
}

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
	// Link requests are handled without requiring SetPacketCallback.
	d.Receive(&packet.Packet{PacketType: packet.PacketTypeLinkReq}, nil)
}

func TestReceiveLinkRequestWithoutPacketCallback(t *testing.T) {
	handlerCalled := false
	RegisterIncomingLinkHandler(func(pkt *packet.Packet, dest *Destination, transport any, iface common.NetworkInterface) (any, error) {
		handlerCalled = true
		return struct{}{}, nil
	})
	defer RegisterIncomingLinkHandler(nil)

	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})
	d.Receive(&packet.Packet{PacketType: packet.PacketTypeLinkReq}, nil)
	if !handlerCalled {
		t.Fatal("link request should be handled without a packet callback")
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

func TestNew_RejectsDotsInAppName(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr := &mockTransport{}
	_, err = New(id, In|Out, Single, "bad.app", tr, "aspect")
	if err == nil || !strings.Contains(err.Error(), "app names") {
		t.Fatalf("expected dots-in-app-name rejection, got: %v", err)
	}
}

func TestNew_RejectsDotsInAspect(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr := &mockTransport{}
	_, err = New(id, In|Out, Single, "app", tr, "bad.aspect")
	if err == nil || !strings.Contains(err.Error(), "aspects") {
		t.Fatalf("expected dots-in-aspect rejection, got: %v", err)
	}
}

func TestValidateNameParts(t *testing.T) {
	if err := ValidateNameParts("app", "one", "two"); err != nil {
		t.Fatalf("honest name rejected: %v", err)
	}
	if err := ValidateNameParts("a.b"); err == nil {
		t.Fatal("expected app name rejection")
	}
	if err := ValidateNameParts("app", "x.y"); err == nil {
		t.Fatal("expected aspect rejection")
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

func TestNewRequiresTransportForIn(t *testing.T) {
	id, _ := identity.New()
	_, err := New(id, In, Single, "app", nil)
	if !errors.Is(err, common.ErrDestTransportRequiredForIn) {
		t.Fatalf("got %v, want ErrDestTransportRequiredForIn", err)
	}
	if _, err := New(id, Out, Single, "app", nil); err != nil {
		t.Fatalf("Out destination with nil transport should succeed: %v", err)
	}
}

func TestAnnounceFailsWithNoInterfaces(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	tr := &mockTransport{
		config:     &common.ReticulumConfig{},
		interfaces: map[string]common.NetworkInterface{},
	}
	dest, err := New(id, In|Out, Single, "testapp", tr, "aspect")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = dest.Announce(false, nil, nil)
	if !errors.Is(err, common.ErrDestAnnounceNoInterfaces) {
		t.Fatalf("got %v, want ErrDestAnnounceNoInterfaces", err)
	}
}

func TestAnnounceFailsWhenAllInterfacesUnusable(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	offline := newRecordingInterface("offline")
	offline.online = false
	tr := &mockTransport{
		config: &common.ReticulumConfig{},
		interfaces: map[string]common.NetworkInterface{
			"offline": offline,
		},
	}
	dest, err := New(id, In|Out, Single, "testapp", tr, "aspect")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = dest.Announce(false, nil, nil)
	if !errors.Is(err, common.ErrDestAnnounceNoWritable) {
		t.Fatalf("got %v, want ErrDestAnnounceNoWritable", err)
	}
}

func TestAnnounceRequiresTransport(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	dest, err := New(id, Out, Single, "testapp", nil, "aspect")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = dest.Announce(false, nil, nil)
	if !errors.Is(err, common.ErrDestTransportNotSet) {
		t.Fatalf("got %v, want ErrDestTransportNotSet", err)
	}
}

func TestHandleIncomingLinkRequestMissingHandlerMessage(t *testing.T) {
	RegisterIncomingLinkHandler(nil)
	id, _ := identity.New()
	d, _ := New(id, In, Single, "app", &mockTransport{})
	err := d.HandleIncomingLinkRequest(&packet.Packet{PacketType: packet.PacketTypeLinkReq}, nil, nil)
	if !errors.Is(err, common.ErrDestNoIncomingLinkHandler) {
		t.Fatalf("got %v, want ErrDestNoIncomingLinkHandler", err)
	}
	if !strings.Contains(err.Error(), "pkg/link") {
		t.Fatalf("error should mention pkg/link import: %v", err)
	}
}

func TestParseName(t *testing.T) {
	app, aspects, err := ParseName("app.aspect.extra")
	if err != nil {
		t.Fatalf("ParseName: %v", err)
	}
	if app != "app" || len(aspects) != 2 || aspects[0] != "aspect" || aspects[1] != "extra" {
		t.Fatalf("got app=%q aspects=%v", app, aspects)
	}
	app, aspects, err = ParseName("solo")
	if err != nil || app != "solo" || aspects != nil {
		t.Fatalf("solo: app=%q aspects=%v err=%v", app, aspects, err)
	}
	if _, _, err := ParseName(""); err == nil {
		t.Fatal("empty name should error")
	}
	if _, _, err := ParseName("   "); err == nil {
		t.Fatal("whitespace-only name should error")
	}
}
