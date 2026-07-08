// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataDirFromConfigPath(t *testing.T) {
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config")
	dir, err := DataDir(cfg)
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	want := filepath.Join(tmp, "storage")
	if dir != want {
		t.Fatalf("DataDir = %q, want %q", dir, want)
	}
}

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "destination_table")
	data := []byte{0x91, 0x01}

	if err := AtomicWriteFile(path, data, 0o600); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("content mismatch: %v", got)
	}
}
