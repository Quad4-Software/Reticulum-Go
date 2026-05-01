// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"os"
	"path/filepath"
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/internal/config"
	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
)

func TestNewReticulum(t *testing.T) {
	// Set up a temporary home directory for testing
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	cfg := config.DefaultConfig()
	// Disable interfaces for simple test
	cfg.Interfaces = make(map[string]*common.InterfaceConfig)

	r, err := NewReticulum(cfg)
	if err != nil {
		t.Fatalf("NewReticulum failed: %v", err)
	}
	if r == nil {
		t.Fatal("NewReticulum returned nil")
	}

	if r.transport == nil {
		t.Error("Reticulum transport should not be nil")
	}

	// Verify directories were created
	basePath := filepath.Join(tmpDir, ".reticulum-go")
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		t.Error("Base directory not created")
	}
}
