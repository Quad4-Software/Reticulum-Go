// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package sandbox

import (
	"runtime"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestApplyStrictFailsWhenMechanismsSkipped(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	t.Setenv("RETICULUM_QEMU_USER", "1")
	err := Apply(&common.ReticulumConfig{
		EnableSandbox: true,
		EnableSeccomp: true,
		SandboxStrict: true,
	})
	if err == nil {
		t.Fatal("sandbox_strict should fail when landlock is skipped")
	}
	if !strings.Contains(err.Error(), "qemu-user") && !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := Apply(&common.ReticulumConfig{
		EnableSandbox: true,
		EnableSeccomp: true,
		SandboxStrict: false,
	}); err != nil {
		t.Fatalf("non-strict apply should continue: %v", err)
	}
}

func TestSetExecRlimitsToggle(t *testing.T) {
	SetExecRlimits(true)
	if !execRlimits.Load() {
		t.Fatal("expected enabled")
	}
	SetExecRlimits(false)
	if execRlimits.Load() {
		t.Fatal("expected disabled")
	}
}
