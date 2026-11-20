// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteHeapProfileBounded(t *testing.T) {
	dir := t.TempDir()
	path, err := writeHeapProfileBounded(dir)
	if err != nil {
		t.Fatalf("writeHeapProfileBounded: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		t.Fatalf("profile not written: %v %v", path, err)
	}
	for range heapProfileKeep + 3 {
		if _, err := writeHeapProfileBounded(dir); err != nil {
			t.Fatalf("rewrite: %v", err)
		}
	}
	matches, err := filepath.Glob(filepath.Join(dir, "heap-*.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != heapProfileKeep {
		t.Fatalf("profiles kept = %d, want %d", len(matches), heapProfileKeep)
	}
}
