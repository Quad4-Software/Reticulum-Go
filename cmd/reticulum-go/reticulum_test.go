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

	if r.identity == nil {
		t.Error("Reticulum identity should not be nil")
	}
	if r.destination == nil {
		t.Error("Reticulum destination should not be nil")
	}

	// Verify directories were created
	basePath := filepath.Join(tmpDir, ".reticulum-go")
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		t.Error("Base directory not created")
	}
}

func TestNodeAppData(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	r := &Reticulum{
		nodeEnabled:     true,
		maxTransferSize: 500,
	}

	data := r.createNodeAppData()
	if len(data) == 0 {
		t.Error("createNodeAppData returned empty data")
	}
	if data[0] != 0x93 {
		t.Errorf("Expected array header 0x93, got 0x%x", data[0])
	}
}
