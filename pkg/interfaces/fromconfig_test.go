// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestNewFromConfigUnsupported(t *testing.T) {
	_, err := NewFromConfig("x", &common.InterfaceConfig{Type: "NoSuchInterface", Enabled: true})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported type error, got %v", err)
	}
}

func TestNewFromConfigNil(t *testing.T) {
	_, err := NewFromConfig("x", nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}
