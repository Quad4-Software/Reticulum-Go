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
}
