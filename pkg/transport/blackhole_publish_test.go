// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"

	"quad4/reticulum-go/pkg/blackhole"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
)

func TestInitializeBlackholePublish(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	cfg := common.DefaultConfig()
	cfg.PublishBlackhole = true
	cfg.InMemoryStorage = true
	tr := NewTransport(cfg)
	tr.mutex.Lock()
	tr.setTransportIdentityLocked(id)
	tr.rpcIdentity = id
	tr.mutex.Unlock()
	blackhole.SetLocalIdentityHash(id.Hash())
	tab := blackhole.New("")
	if _, err := tab.Add(bytes.Repeat([]byte{0x11}, 16), 0, "test"); err != nil {
		t.Fatal(err)
	}
	tr.SetBlackholeTable(tab)

	if err := tr.InitializeBlackholePublish(); err != nil {
		t.Fatalf("InitializeBlackholePublish: %v", err)
	}
	dest := tr.BlackholePublishDestination()
	if dest == nil {
		t.Fatal("expected blackhole destination")
	}
	got := dest.HandleRequest("/list", nil, nil, nil, nil, 0)
	decoded, err := blackhole.DecodeBlackholeMap(got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("entries: got %d want 1", len(decoded))
	}
	if err := tr.InitializeBlackholePublish(); err != nil {
		t.Fatalf("idempotent init: %v", err)
	}
}

func TestInitializeBlackholePublishDisabled(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.InMemoryStorage = true
	tr := NewTransport(cfg)
	if err := tr.InitializeBlackholePublish(); err != nil {
		t.Fatal(err)
	}
	if tr.BlackholePublishDestination() != nil {
		t.Fatal("expected nil when publish_blackhole off")
	}
}
