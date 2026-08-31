// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/profiler"
)

func TestInitializeRemoteManagementRegistersPathHandler(t *testing.T) {
	allowed, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &common.ReticulumConfig{
		EnableTransport:         true,
		InMemoryStorage:         true,
		EnableRemoteManagement:  true,
		RemoteManagementAllowed: [][]byte{allowed.Hash()},
	}
	tr := NewTransport(cfg)
	defer func() { _ = tr.Close() }()

	if err := tr.InitializeRemoteManagement(); err != nil {
		t.Fatal(err)
	}
	dest := tr.RemoteManagementDestination()
	if dest == nil {
		t.Fatal("expected remote management destination")
	}
	wantHash := destination.Hash(tr.TransportIdentity(), "rnstransport", "remote", "management")
	if !bytes.Equal(dest.GetHash(), wantHash) {
		t.Fatalf("dest hash %x want %x", dest.GetHash(), wantHash)
	}

	packed, err := msgpack.Marshal([]any{"table", nil, nil})
	if err != nil {
		t.Fatal(err)
	}
	denied := dest.HandleRequest("/path", packed, nil, nil, stranger, 0)
	if !bytes.Contains(denied, []byte("Not Found")) {
		t.Fatalf("stranger should be denied, got %q", denied)
	}
	ok := dest.HandleRequest("/path", packed, nil, nil, allowed, 0)
	if bytes.Contains(ok, []byte("Not Found")) {
		t.Fatalf("allowed identity denied: %q", ok)
	}
	var table []any
	if err := msgpack.Unmarshal(ok, &table); err != nil {
		t.Fatalf("unmarshal table: %v %q", err, ok)
	}

	statusPacked, err := msgpack.Marshal([]any{false})
	if err != nil {
		t.Fatal(err)
	}
	statusRaw := dest.HandleRequest("/status", statusPacked, nil, nil, allowed, 0)
	var status []any
	if err := msgpack.Unmarshal(statusRaw, &status); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if len(status) != 1 {
		t.Fatalf("status without links: len %d", len(status))
	}

	profiler.Reset()
	defer profiler.Reset()
	profiler.Do("remote.status.probe", func() {})
	statusPacked2, err := msgpack.Marshal([]any{true, true})
	if err != nil {
		t.Fatal(err)
	}
	statusRaw2 := dest.HandleRequest("/status", statusPacked2, nil, nil, allowed, 0)
	var status2 []any
	if err := msgpack.Unmarshal(statusRaw2, &status2); err != nil {
		t.Fatalf("unmarshal status+prof: %v", err)
	}
	if len(status2) < 3 {
		t.Fatalf("status with links+profiling: len %d", len(status2))
	}
	if status2[2] == nil {
		t.Fatal("expected profiling results in remote /status")
	}
}

func TestInitializeRemoteManagementDisabled(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true, InMemoryStorage: true})
	defer func() { _ = tr.Close() }()
	if err := tr.InitializeRemoteManagement(); err != nil {
		t.Fatal(err)
	}
	if tr.RemoteManagementDestination() != nil {
		t.Fatal("expected no destination when disabled")
	}
}
