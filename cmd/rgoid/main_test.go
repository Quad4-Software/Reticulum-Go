// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/cli"
)

func TestRgoidGeneratePrintSignVerify(t *testing.T) {
	dir := t.TempDir()
	idPath := filepath.Join(dir, "test.rid")
	msgPath := filepath.Join(dir, "msg.bin")
	if err := os.WriteFile(msgPath, []byte("probe payload"), 0o600); err != nil {
		t.Fatal(err)
	}

	if code := cli.RunID([]string{"-g", idPath, "-p"}); code != 0 {
		t.Fatalf("generate+print exit %d", code)
	}
	if code := cli.RunID([]string{"-i", idPath, "-s", msgPath, "-f"}); code != 0 {
		t.Fatalf("sign exit %d", code)
	}
	sigPath := msgPath + ".rsg"
	if _, err := os.Stat(sigPath); err != nil {
		t.Fatal(err)
	}
	if code := cli.RunID([]string{"-i", idPath, "-V", msgPath}); code != 0 {
		t.Fatalf("verify exit %d", code)
	}
	if code := cli.RunID([]string{"-i", idPath, "-H", "rns.id"}); code != 0 {
		t.Fatalf("hash exit %d", code)
	}

	rsmPath := filepath.Join(dir, "note.rsm")
	if code := cli.RunID([]string{"-i", idPath, "-S", "hello rsm", "-w", rsmPath, "-f"}); code != 0 {
		t.Fatalf("sign message exit %d", code)
	}
	if code := cli.RunID([]string{"-V", rsmPath}); code != 0 {
		t.Fatalf("verify rsm exit %d", code)
	}

	encPath := msgPath + ".rfe"
	if code := cli.RunID([]string{"-i", idPath, "-e", msgPath, "-w", encPath, "-f"}); code != 0 {
		t.Fatalf("encrypt exit %d", code)
	}
	decPath := filepath.Join(dir, "msg.out")
	if code := cli.RunID([]string{"-i", idPath, "-d", encPath, "-w", decPath, "-f"}); code != 0 {
		t.Fatalf("decrypt exit %d", code)
	}
}

func TestRgoidMutualExclusive(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.rid")
	b := filepath.Join(dir, "b.rid")
	if code := cli.RunID([]string{"-g", a, "-i", b}); code == 0 {
		t.Fatal("expected failure for mutual exclusive flags")
	}
}

func TestRgoidHelp(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	code := cli.RunID(nil)
	_ = w.Close()
	os.Stderr = old
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	if code != 2 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(string(buf[:n]), "Usage") && !strings.Contains(string(buf[:n]), "usage") {
		t.Fatalf("help output=%q", string(buf[:n]))
	}
}
