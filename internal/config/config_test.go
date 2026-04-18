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

func TestLoadConfig_SpamProtectionKnobs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := "" +
		"[noisy]\n" +
		"type = TCPClientInterface\n" +
		"enabled = true\n" +
		"announce_cap = 5.5\n" +
		"announce_rate_target = 1800\n" +
		"announce_rate_grace = 4\n" +
		"announce_rate_penalty = 600\n" +
		"ingress_control = no\n" +
		"ic_new_time = 60\n" +
		"ic_burst_freq_new = 1.25\n" +
		"ic_burst_freq = 7\n" +
		"ic_max_held_announces = 64\n" +
		"ic_burst_hold = 30\n" +
		"ic_burst_penalty = 90\n" +
		"ic_held_release_interval = 15\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	iface, ok := cfg.Interfaces["noisy"]
	if !ok {
		t.Fatalf("interface section not parsed")
	}
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"AnnounceCap", iface.AnnounceCap, 5.5},
		{"AnnounceRateTarget", iface.AnnounceRateTarget, 1800.0},
		{"AnnounceRateGrace", iface.AnnounceRateGrace, 4},
		{"AnnounceRatePenalty", iface.AnnounceRatePenalty, 600.0},
		{"IngressControl", iface.IngressControl, false},
		{"IngressControlSet", iface.IngressControlSet, true},
		{"ICNewTime", iface.ICNewTime, 60},
		{"ICBurstFreqNew", iface.ICBurstFreqNew, 1.25},
		{"ICBurstFreq", iface.ICBurstFreq, 7.0},
		{"ICMaxHeldAnnounces", iface.ICMaxHeldAnnounces, 64},
		{"ICBurstHold", iface.ICBurstHold, 30},
		{"ICBurstPenalty", iface.ICBurstPenalty, 90},
		{"ICHeldReleaseInterval", iface.ICHeldReleaseInterval, 15},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestParseValue(t *testing.T) {
	tests := []struct {
		input    string
		expected any
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
	if loadedCfg.EnableTransport {
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
