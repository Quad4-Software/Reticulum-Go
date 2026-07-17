// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"runtime"
	"testing"
)

func TestDefaultSharedInstanceType(t *testing.T) {
	got := DefaultSharedInstanceType()
	want := SharedInstanceTCP
	if runtime.GOOS == "linux" {
		want = SharedInstanceUnix
	}
	if got != want {
		t.Fatalf("DefaultSharedInstanceType() = %q, want %q", got, want)
	}
}

func TestResolveSharedInstanceType(t *testing.T) {
	if got := ResolveSharedInstanceType("tcp"); got != SharedInstanceTCP {
		t.Fatalf("tcp: got %q", got)
	}
	if got := ResolveSharedInstanceType("UNIX"); got != SharedInstanceUnix {
		t.Fatalf("UNIX: got %q", got)
	}
	if got := ResolveSharedInstanceType(""); got != DefaultSharedInstanceType() {
		t.Fatalf("empty: got %q want %q", got, DefaultSharedInstanceType())
	}
	if got := ResolveSharedInstanceType("bogus"); got != DefaultSharedInstanceType() {
		t.Fatalf("bogus: got %q want %q", got, DefaultSharedInstanceType())
	}
}

func TestNewReticulumConfigSharedInstanceType(t *testing.T) {
	cfg := NewReticulumConfig()
	if cfg.SharedInstanceType != DefaultSharedInstanceType() {
		t.Fatalf("SharedInstanceType = %q, want %q", cfg.SharedInstanceType, DefaultSharedInstanceType())
	}
}
