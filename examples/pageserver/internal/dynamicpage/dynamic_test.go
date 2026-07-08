// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package dynamicpage

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadOrExecuteShebangExecutableRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shebang execution is not supported the same way on windows")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "prop_script.mu")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho 'script output'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := ReadOrExecute(p, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.TrimSpace(out); !bytes.Equal(got, []byte("script output")) {
		t.Fatalf("got %q want script output", got)
	}
}

func TestReadOrExecuteShebangNotExecutableServesRaw(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "prop_script.mu")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := ReadOrExecute(p, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("#!/bin/sh")) {
		t.Fatalf("expected raw file, got %q", out)
	}
}

func TestReadOrExecuteExecutableNoShebangServesRaw(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "plain.mu")
	if err := os.WriteFile(p, []byte("plain text\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := ReadOrExecute(p, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, []byte("plain text\n")) {
		t.Fatalf("got %q", out)
	}
}

func TestReadOrExecuteNonMuShebangExecutableIgnored(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shebang execution is not supported the same way on windows")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho x\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := ReadOrExecute(p, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("#!/bin/sh")) {
		t.Fatalf("expected raw file for non-.mu, got %q", out)
	}
}
