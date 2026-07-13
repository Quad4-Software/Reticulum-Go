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
