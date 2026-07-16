// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sandbox

import (
	"runtime"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestApply_Disabled(t *testing.T) {
	cfg := &common.ReticulumConfig{EnableSandbox: false}
	if err := Apply(cfg); err != nil {
		t.Fatalf("Apply with disabled sandbox should return nil, got %v", err)
	}
}

func TestApply_NilConfig(t *testing.T) {
	switch runtime.GOOS {
	case "linux":
		// On Linux Apply triggers Landlock which persists for the process
		// lifetime and would break later tests in the same binary.
		// The full Linux path is exercised in TestLandlockFunctional.
		t.Skip("Linux Apply path tested via TestLandlockFunctional")
	case "freebsd", "openbsd":
		// CapEnter / pledge persist for the process and would break later tests.
		t.Skip("Apply enters capability mode and would break the test process")
	}
	// nil config defaults to enabled. Should not panic and should apply
	// platform-specific restrictions (or no-op on unsupported platforms).
	if err := Apply(nil); err != nil {
		t.Fatalf("Apply with nil config should return nil, got %v", err)
	}
}

func TestApply_Enabled(t *testing.T) {
	switch runtime.GOOS {
	case "linux":
		t.Skip("Linux Apply path tested via TestLandlockFunctional")
	case "freebsd", "openbsd":
		t.Skip("Apply enters capability mode and would break the test process")
	}
	cfg := &common.ReticulumConfig{EnableSandbox: true}
	if err := Apply(cfg); err != nil {
		t.Fatalf("Apply with enabled sandbox should return nil, got %v", err)
	}
}
