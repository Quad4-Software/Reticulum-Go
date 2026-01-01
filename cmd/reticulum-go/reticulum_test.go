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
	originalHome := os.Getenv(common.STR_HOME)
	os.Setenv(common.STR_HOME, tmpDir)
	defer os.Setenv(common.STR_HOME, originalHome)

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
	os.Setenv(common.STR_HOME, tmpDir)

	r := &Reticulum{
		nodeEnabled:     true,
		maxTransferSize: common.NUM_500,
	}

	data := r.createNodeAppData()
	if len(data) == common.ZERO {
		t.Error("createNodeAppData returned empty data")
	}
	if data[0] != common.HEX_0x93 {
		t.Errorf("Expected array header 0x93, got 0x%x", data[0])
	}
}
