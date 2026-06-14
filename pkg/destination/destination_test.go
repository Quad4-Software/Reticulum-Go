// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/announce"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
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
// one interface has flapped offline; it must still emit on every
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
