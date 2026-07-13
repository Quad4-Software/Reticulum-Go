// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/transport"
)

func TestParseDestHash(t *testing.T) {
	h, err := ParseDestHash("aabbccddeeff00112233445566778899")
	if err != nil || len(h) != 16 {
		t.Fatalf("%v %x", err, h)
	}
	if _, err := ParseDestHash("short"); err == nil {
		t.Fatal("expected error")
	}
}

func TestWritePathTableJSON(t *testing.T) {
	var buf bytes.Buffer
	err := WritePathTableJSON(&buf, []transport.PathTableEntry{{
		Hash:      []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Via:       []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		Hops:      2,
		Expires:   1,
		Interface: "UDP",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"hops":2`)) {
		t.Fatalf("%s", buf.String())
	}
}

func TestUniqueSavePath(t *testing.T) {
	dir := t.TempDir()
	p1, err := UniqueSavePath(dir, "f.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p1, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	p2, err := UniqueSavePath(dir, "f.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if p2 == p1 {
		t.Fatal("expected unique path")
	}
	if filepath.Base(p2) != "f.txt.1" {
		t.Fatalf("got %s", p2)
	}
}

func TestClassifyFetchResponse(t *testing.T) {
	if ClassifyFetchResponse(true) != FetchFound {
		t.Fatal("true")
	}
	if ClassifyFetchResponse(false) != FetchNotFound {
		t.Fatal("false")
	}
	if ClassifyFetchResponse(nil) != FetchRemoteError {
		t.Fatal("nil")
	}
	if ClassifyFetchResponse(RNCPFetchNotAllowed) != FetchNotAllowed {
		t.Fatal("0xf0")
	}
}

func TestFilenameFromMetadata(t *testing.T) {
	if FilenameFromMetadata(map[string]any{"name": []byte("/tmp/x.bin")}) != "x.bin" {
		t.Fatal("basename")
	}
}
