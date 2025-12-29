package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config")

	configContent := `[identity]
name = test-identity
storage_path = /tmp/test-storage

[transport]
announce_interval = 300
path_request_timeout = 15
max_hops = 8
bitrate_limit = 1000000

[logging]
level = info
file = /tmp/test.log

[interface test-interface]
type = UDPInterface
enabled = true
listen_ip = 127.0.0.1
listen_port = 37696
`

	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("LoadConfig() returned nil")
	}

	if len(cfg.Interfaces) == 0 {
		t.Error("No interfaces loaded")
	}

	iface := cfg.Interfaces[0]
	if iface.Type != "UDPInterface" {
		t.Errorf("Interface type = %s, want UDPInterface", iface.Type)
	}
	if !iface.Enabled {
		t.Error("Interface should be enabled")
	}
	if iface.ListenIP != "127.0.0.1" {
		t.Errorf("Interface ListenIP = %s, want 127.0.0.1", iface.ListenIP)
	}
	if iface.ListenPort != 37696 {
		t.Errorf("Interface ListenPort = %d, want 37696", iface.ListenPort)
	}
}

func TestLoadConfig_NonexistentFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config")
	if err == nil {
		t.Error("LoadConfig() should return error for nonexistent file")
	}
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "empty_config")

	if err := os.WriteFile(configPath, []byte(""), 0600); err != nil {
		t.Fatalf("Failed to write empty config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("LoadConfig() returned nil")
	}
}

func TestLoadConfig_CommentsAndEmptyLines(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config")

	configContent := `# Comment line

[identity]
name = test
# Another comment

[interface test-interface]
# Interface comment
type = UDPInterface
enabled = true
`

	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("LoadConfig() returned nil")
	}

	if cfg.Identity.Name != "test" {
		t.Errorf("Identity.Name = %s, want test", cfg.Identity.Name)
	}
}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config")

	cfg := &Config{}
	cfg.Identity.Name = "test-identity"
	cfg.Identity.StoragePath = "/tmp/test"
	cfg.Transport.AnnounceInterval = 600
	cfg.Logging.Level = "debug"
	cfg.Logging.File = "/tmp/test.log"

	if err := SaveConfig(cfg, configPath); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if loaded.Identity.Name != "test-identity" {
		t.Errorf("Identity.Name = %s, want test-identity", loaded.Identity.Name)
	}
	if loaded.Transport.AnnounceInterval != 600 {
		t.Errorf("Transport.AnnounceInterval = %d, want 600", loaded.Transport.AnnounceInterval)
	}
}

func TestGetConfigDir(t *testing.T) {
	dir := GetConfigDir()
	if dir == "" {
		t.Error("GetConfigDir() returned empty string")
	}
}

func TestGetDefaultConfigPath(t *testing.T) {
	path := GetDefaultConfigPath()
	if path == "" {
		t.Error("GetDefaultConfigPath() returned empty string")
	}
}

func TestEnsureConfigDir(t *testing.T) {
	if err := EnsureConfigDir(); err != nil {
		t.Fatalf("EnsureConfigDir() error = %v", err)
	}
}

func TestInitConfig(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	defer func() {
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		}
	}()

	os.Setenv("HOME", tmpDir)

	cfg, err := InitConfig()
	if err != nil {
		t.Fatalf("InitConfig() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("InitConfig() returned nil")
	}
}
