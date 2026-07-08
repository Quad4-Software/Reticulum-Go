// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package config

import (
	"os"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/reticulumconfig"
)

// withTempHome points HOME at an isolated temp directory for the duration of
// the test so EnsureConfigDir / InitConfig never touch the real user home.
func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	backup, hadHome := os.LookupEnv("HOME")
	t.Setenv("HOME", home)
	t.Cleanup(func() {
		if hadHome {
			_ = os.Setenv("HOME", backup)
		} else {
			_ = os.Unsetenv("HOME")
		}
	})
	return home
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}
	if !cfg.EnableTransport {
		t.Error("DefaultConfig should enable transport")
	}
	if cfg.SharedInstancePort != reticulumconfig.DefaultSharedInstancePort {
		t.Errorf("SharedInstancePort: got %d, want %d",
			cfg.SharedInstancePort, reticulumconfig.DefaultSharedInstancePort)
	}
}

func TestDefaultConfigConstantsMatchReticulumConfig(t *testing.T) {
	cases := []struct {
		name string
		got  any
		want any
	}{
		{"DefaultSharedInstancePort", DefaultSharedInstancePort, reticulumconfig.DefaultSharedInstancePort},
		{"DefaultInstanceControlPort", DefaultInstanceControlPort, reticulumconfig.DefaultInstanceControlPort},
		{"DefaultLogLevel", DefaultLogLevel, reticulumconfig.DefaultLogLevel},
		{"DefaultConfigDirName", DefaultConfigDirName, reticulumconfig.DefaultConfigDirName},
		{"DefaultConfigFileName", DefaultConfigFileName, reticulumconfig.DefaultConfigFileName},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestGetConfigPath(t *testing.T) {
	home := withTempHome(t)
	got, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath: %v", err)
	}
	want := filepath.Join(home, DefaultConfigDirName, DefaultConfigFileName)
	if got != want {
		t.Errorf("GetConfigPath: got %q, want %q", got, want)
	}
}

func TestGetConfigPath_NoHome(t *testing.T) {
	t.Setenv("HOME", "")
	if _, err := GetConfigPath(); err == nil {
		t.Fatal("expected error when HOME is unset")
	}
}

func TestEnsureConfigDir(t *testing.T) {
	home := withTempHome(t)
	if err := EnsureConfigDir(); err != nil {
		t.Fatalf("EnsureConfigDir: %v", err)
	}
	dir := filepath.Join(home, DefaultConfigDirName)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("config dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", dir)
	}
	// Calling again on an existing directory must be a no-op success.
	if err := EnsureConfigDir(); err != nil {
		t.Fatalf("EnsureConfigDir second call: %v", err)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope")
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error loading missing file")
	}
}

func TestSaveConfig_RoundTripViaWrapper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	cfg := DefaultConfig()
	cfg.ConfigPath = path
	cfg.LogLevel = 3

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.LogLevel != 3 {
		t.Errorf("LogLevel round-trip: got %d, want 3", loaded.LogLevel)
	}
	if loaded.ConfigPath != path {
		t.Errorf("ConfigPath round-trip: got %q, want %q", loaded.ConfigPath, path)
	}
}

func TestSaveConfig_RequiresPath(t *testing.T) {
	if err := SaveConfig(&common.ReticulumConfig{}); err == nil {
		t.Fatal("expected error when ConfigPath unset")
	}
	if err := SaveConfig(nil); err == nil {
		t.Fatal("expected error when cfg is nil")
	}
}

func TestCreateDefaultConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := CreateDefaultConfig(path); err != nil {
		t.Fatalf("CreateDefaultConfig: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	auto, ok := cfg.Interfaces["Auto Discovery"]
	if !ok {
		t.Fatal("default Auto Discovery interface missing")
	}
	if !auto.Enabled {
		t.Error("Auto Discovery should be enabled by default")
	}
}

func TestInitConfig_CreatesDefault(t *testing.T) {
	withTempHome(t)
	cfg, err := InitConfig()
	if err != nil {
		t.Fatalf("InitConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("InitConfig returned nil config")
	}
	gotPath, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath: %v", err)
	}
	if _, err := os.Stat(gotPath); err != nil {
		t.Fatalf("config file not created by InitConfig: %v", err)
	}
}

func TestInitConfig_LoadsExisting(t *testing.T) {
	withTempHome(t)

	// First call creates defaults.
	first, err := InitConfig()
	if err != nil {
		t.Fatalf("InitConfig first: %v", err)
	}
	first.LogLevel = 5
	if err := SaveConfig(first); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// Second call must load the persisted file, not overwrite it.
	second, err := InitConfig()
	if err != nil {
		t.Fatalf("InitConfig second: %v", err)
	}
	if second.LogLevel != 5 {
		t.Errorf("expected existing LogLevel 5, got %d", second.LogLevel)
	}
}
