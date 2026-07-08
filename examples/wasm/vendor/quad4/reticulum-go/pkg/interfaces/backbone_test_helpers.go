// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"testing"

	"quad4/reticulum-go/pkg/backbone"
)

func testBackboneHub(t *testing.T) *backbone.Hub {
	t.Helper()
	hub, err := backbone.Init(backbone.BackendGo)
	if err != nil {
		t.Fatalf("backbone.Init: %v", err)
	}
	t.Cleanup(backbone.Shutdown)
	return hub
}
