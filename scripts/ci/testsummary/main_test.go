// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"slices"
	"testing"
)

func TestGoTestArgs_ChdirFirst(t *testing.T) {
	got := goTestArgs([]string{"-C", "examples/wasm", "-v", "."})
	want := []string{"test", "-C", "examples/wasm", "-json", "-v", "."}
	if !slices.Equal(got, want) {
		t.Fatalf("goTestArgs() = %#v, want %#v", got, want)
	}
}

func TestGoTestArgs_ChdirEquals(t *testing.T) {
	got := goTestArgs([]string{"-C=examples/wasm", "-v", "."})
	want := []string{"test", "-C=examples/wasm", "-json", "-v", "."}
	if !slices.Equal(got, want) {
		t.Fatalf("goTestArgs() = %#v, want %#v", got, want)
	}
}

func TestGoTestArgs_NoChdir(t *testing.T) {
	got := goTestArgs([]string{"-v", "./pkg/wasm/"})
	want := []string{"test", "-json", "-v", "./pkg/wasm/"}
	if !slices.Equal(got, want) {
		t.Fatalf("goTestArgs() = %#v, want %#v", got, want)
	}
}

func TestValidateGoTestArgs(t *testing.T) {
	if err := validateGoTestArgs([]string{"-v", "./...", "-count=1"}); err != nil {
		t.Fatalf("valid args rejected: %v", err)
	}
	if err := validateGoTestArgs([]string{"-C", "examples/wasm", "-exec=/usr/lib/go/lib/wasm/go_js_wasm_exec"}); err != nil {
		t.Fatalf("chdir and exec args rejected: %v", err)
	}
	if err := validateGoTestArgs([]string{"-v;id"}); err == nil {
		t.Fatal("shell metacharacter was accepted")
	}
	if err := validateGoTestArgs([]string{"ok\n-v"}); err == nil {
		t.Fatal("newline was accepted")
	}
	if err := validateGoTestArgs([]string{"-run", "TestSimChaos|TestLinkChaos|TestIfaceChaos"}); err != nil {
		t.Fatalf("-run alternation rejected: %v", err)
	}
	if err := validateGoTestArgs([]string{"-run", "^$", "-fuzz=^FuzzPacketUnpack$"}); err != nil {
		t.Fatalf("fuzz skip-unit-tests args rejected: %v", err)
	}
}

func TestIsSpuriousFuzzDeadline(t *testing.T) {
	pkg := "quad4/reticulum-go/pkg/packet"
	failed := map[string]map[string]struct{}{
		pkg: {"FuzzReadPCAPUDPPayloads": {}},
	}
	outputs := map[string]map[string][]string{
		pkg: {"FuzzReadPCAPUDPPayloads": {"    context deadline exceeded\n"}},
	}
	pkgOut := map[string][]string{
		pkg: {"fuzz: elapsed: 20s, execs: 100 (0/sec), new interesting: 0 (total: 7)\n"},
	}
	if !isSpuriousFuzzDeadline(failed, nil, outputs, pkgOut) {
		t.Fatal("expected spurious fuzz deadline")
	}

	outputs[pkg]["FuzzReadPCAPUDPPayloads"] = []string{"    pcap_fuzz_test.go:48: incl len absurd\n"}
	if isSpuriousFuzzDeadline(failed, nil, outputs, pkgOut) {
		t.Fatal("file:line failure must not be spurious")
	}

	outputs[pkg]["FuzzReadPCAPUDPPayloads"] = []string{"    context deadline exceeded\n"}
	delete(pkgOut, pkg)
	if isSpuriousFuzzDeadline(failed, nil, outputs, pkgOut) {
		t.Fatal("missing fuzz progress must not be spurious")
	}

	failed[pkg]["TestFoo"] = struct{}{}
	if isSpuriousFuzzDeadline(failed, nil, outputs, map[string][]string{pkg: pkgOut[pkg]}) {
		t.Fatal("non-fuzz test failure must not be spurious")
	}
}
