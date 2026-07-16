// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package blackhole

import (
	"bytes"
	"testing"
)

func TestPersistLocal_EmptyDirIsNoop(t *testing.T) {
	tab := New("")
	local := bytes.Repeat([]byte{0x11}, HashLen)
	target := bytes.Repeat([]byte{0x22}, HashLen)
	SetLocalIdentityHash(local)
	added, err := tab.Add(target, 0, "test")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !added {
		t.Fatal("expected add")
	}
	if err := tab.PersistLocal(); err != nil {
		t.Fatalf("PersistLocal empty dir: %v", err)
	}
	if !tab.Has(target) {
		t.Fatal("entry should remain in memory")
	}
}
