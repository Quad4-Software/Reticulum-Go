// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package sandbox

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestShouldRestrictScoped(t *testing.T) {
	if !shouldRestrictScoped(nil) {
		t.Fatal("nil config should keep RestrictScoped")
	}
	if !shouldRestrictScoped(&common.ReticulumConfig{}) {
		t.Fatal("default config should keep RestrictScoped")
	}
	if shouldRestrictScoped(&common.ReticulumConfig{SandboxSkipScoped: true}) {
		t.Fatal("SandboxSkipScoped should skip RestrictScoped")
	}
}
