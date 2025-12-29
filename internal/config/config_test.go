package config

import (
	"os"
	"path/filepath"
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}
	if !cfg.EnableTransport {
		t.Error("EnableTransport should be true by default")
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel should be %d, got %d", DefaultLogLevel, cfg.LogLevel)
	}
}

func TestParseValue(t *testing.T) {
	tests := []struct {
		input    string
		expected interface{}
	}{
		{"true", true},
		{"false", false},
		{"123", 123},
		{"hello", "hello"},
		{"  456  ", 456},
		{"  world  ", "world"},
	}

	for _, tt := range tests {
		result := parseValue(tt.input)
		if result != tt.expected {
			t.Errorf("parseValue(%q) = %v; want %v", tt.input, result, tt.expected)
		}
	}
}

func TestLoadSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	cfg := DefaultConfig()
	cfg.ConfigPath = configPath
	cfg.LogLevel = 1
	cfg.EnableTransport = false
	cfg.Interfaces["TestInterface"] = &common.InterfaceConfig{
		Name:    "TestInterface",
		Type:    "UDPInterface",
		Enabled: true,
		Address: "1.2.3.4",
		Port:    1234,
	}

	err := SaveConfig(cfg)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loadedCfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loadedCfg.LogLevel != 1 {
		t.Errorf("Expected LogLevel 1, got %d", loadedCfg.LogLevel)
	}
	if loadedCfg.EnableTransport != false {
		t.Error("Expected EnableTransport false")
	}

	iface, ok := loadedCfg.Interfaces["TestInterface"]
	if !ok {
		t.Fatal("TestInterface not found in loaded config")
	}
	if iface.Type != "UDPInterface" {
		t.Errorf("Expected type UDPInterface, got %s", iface.Type)
	}
	if iface.Address != "1.2.3.4" {
		t.Errorf("Expected address 1.2.3.4, got %s", iface.Address)
	}
	if iface.Port != 1234 {
		t.Errorf("Expected port 1234, got %d", iface.Port)
	}
}

func TestCreateDefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	err := CreateDefaultConfig(configPath)
	if err != nil {
		t.Fatalf("CreateDefaultConfig failed: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if _, ok := cfg.Interfaces["Auto Discovery"]; !ok {
		t.Error("Auto Discovery interface missing")
	}
}

func TestGetConfigPath(t *testing.T) {
	path, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath failed: %v", err)
	}
	if path == "" {
		t.Error("GetConfigPath returned empty string")
	}
}

func TestEnsureConfigDir(t *testing.T) {
	// This might modify the actual home directory if not careful,
	// but EnsureConfigDir uses os.UserHomeDir().
	// For testing purposes, we can't easily mock os.UserHomeDir() without
	// changing the code or environment variables.
	// Since we are in a sandbox, it should be fine.
	err := EnsureConfigDir()
	if err != nil {
		t.Errorf("EnsureConfigDir failed: %v", err)
	}
}
