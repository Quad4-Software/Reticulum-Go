// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"os"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/internal/config"
	"quad4/reticulum-go/pkg/common"
)

func TestNewReticulum(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("RETICULUM_STORAGE_PATH", "")

	cfg := config.DefaultConfig()
	cfg.ConfigPath = filepath.Join(tmpDir, ".reticulum-go", "config")
	cfg.Interfaces = make(map[string]*common.InterfaceConfig)

	r, err := NewReticulum(cfg)
	if err != nil {
		t.Fatalf("NewReticulum failed: %v", err)
	}
	if r == nil {
		t.Fatal("NewReticulum returned nil")
	}
	if r.Transport() == nil {
		t.Error("Reticulum transport should not be nil")
	}

	basePath := filepath.Join(tmpDir, ".reticulum-go")
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		t.Error("Base directory not created")
	}
}

func TestNewReticulum_InMemoryStorageSkipsDirs(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("RETICULUM_STORAGE_PATH", "")

	cfg := config.DefaultConfig()
	cfg.ConfigPath = filepath.Join(tmpDir, ".reticulum-go", "config")
	cfg.InMemoryStorage = true
	cfg.Interfaces = make(map[string]*common.InterfaceConfig)

	r, err := NewReticulum(cfg)
	if err != nil {
		t.Fatalf("NewReticulum: %v", err)
	}
	if r == nil || r.Transport() == nil {
		t.Fatal("expected running in-memory instance")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".reticulum-go")); !os.IsNotExist(err) {
		t.Fatal("in-memory mode must not bootstrap ~/.reticulum-go")
	}
}
