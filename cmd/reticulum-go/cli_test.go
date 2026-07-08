// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLIVersion(t *testing.T) {
	var buf bytes.Buffer
	run, code := parseCLI([]string{"--version"})
	if run || code != 0 {
		t.Fatalf("parseCLI(--version) = run %v code %d", run, code)
	}
	printVersion(&buf)
	if !strings.Contains(buf.String(), "reticulum-go") {
		t.Fatalf("version output: %q", buf.String())
	}
}

func TestCLIHelp(t *testing.T) {
	run, code := parseCLI([]string{"--help"})
	if run || code != 0 {
		t.Fatalf("parseCLI(--help) = run %v code %d", run, code)
	}
	run, code = parseCLI([]string{"-h"})
	if run || code != 0 {
		t.Fatalf("parseCLI(-h) = run %v code %d", run, code)
	}
}

func TestCLIUnknownArg(t *testing.T) {
	run, code := parseCLI([]string{"--bogus"})
	if run || code != 2 {
		t.Fatalf("parseCLI(--bogus) = run %v code %d, want false 2", run, code)
	}
}
