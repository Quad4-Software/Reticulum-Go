// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"bytes"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
)

func TestGroupTokenRoundTrip(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	alice, err := New(id, Out, Group, "groupapp", nil, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if err := alice.CreateKeys(); err != nil {
		t.Fatal(err)
	}
	key, err := alice.GetPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	bob, err := New(id, Out, Group, "groupapp", nil, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if err := bob.LoadPrivateKey(key); err != nil {
		t.Fatal(err)
	}

	plain := []byte("group-secret")
	ct, err := alice.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := bob.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q want %q", got, plain)
	}
	if !bytes.Equal(alice.GetHash(), bob.GetHash()) {
		t.Fatal("group dest hashes must match for the same identity and name")
	}
}

func TestGroupCreateKeysRejectedOnSingle(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := New(id, Out, Single, "groupapp", nil, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if err := dest.CreateKeys(); err == nil {
		t.Fatal("CreateKeys must fail on SINGLE")
	}
}

func TestGroupEncryptRequiresKey(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := New(id, Out, Group, "groupapp", nil, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dest.Encrypt([]byte("x")); err == nil {
		t.Fatal("encrypt without key must fail")
	}
}

func TestGroupAnnounceRejected(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr := &mockTransport{
		config: &common.ReticulumConfig{},
		interfaces: map[string]common.NetworkInterface{
			"if-a": newRecordingInterface("if-a"),
		},
	}
	dest, err := New(id, In, Group, "groupapp", tr, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if err := dest.Announce(false, nil, nil); err == nil {
		t.Fatal("GROUP dests cannot announce")
	}
}
