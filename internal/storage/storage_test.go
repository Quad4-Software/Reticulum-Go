package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	if m == nil {
		t.Fatal("NewManager returned nil")
	}

	expectedBase := filepath.Join(tmpDir, ".reticulum-go", "storage")
	if m.basePath != expectedBase {
		t.Errorf("Expected basePath %s, got %s", expectedBase, m.basePath)
	}

	// Verify directories were created
	dirs := []string{
		m.basePath,
		m.ratchetsPath,
		m.identitiesPath,
		filepath.Join(m.basePath, "cache"),
		filepath.Join(m.basePath, "cache", "announces"),
		filepath.Join(m.basePath, "resources"),
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory %s was not created", dir)
		}
	}
}

func TestSaveLoadRatchets(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	identityHash := []byte("test-identity-hash")
	ratchetKey := make([]byte, 32)
	for i := range ratchetKey {
		ratchetKey[i] = byte(i)
	}

	err = m.SaveRatchet(identityHash, ratchetKey)
	if err != nil {
		t.Fatalf("SaveRatchet failed: %v", err)
	}

	ratchets, err := m.LoadRatchets(identityHash)
	if err != nil {
		t.Fatalf("LoadRatchets failed: %v", err)
	}

	if len(ratchets) != 1 {
		t.Errorf("Expected 1 ratchet, got %d", len(ratchets))
	}

	// The key in the map is the hex of first 16 bytes of ratchetKey
	found := false
	for _, key := range ratchets {
		if bytes.Equal(key, ratchetKey) {
			found = true
			break
		}
	}

	if !found {
		t.Error("Saved ratchet key not found in loaded ratchets")
	}
}

func TestGetters(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	m, _ := NewManager()

	if m.GetBasePath() == "" {
		t.Error("GetBasePath returned empty string")
	}
	if m.GetRatchetsPath() == "" {
		t.Error("GetRatchetsPath returned empty string")
	}
	if m.GetIdentityPath() == "" {
		t.Error("GetIdentityPath returned empty string")
	}
	if m.GetTransportIdentityPath() == "" {
		t.Error("GetTransportIdentityPath returned empty string")
	}
	if m.GetDestinationTablePath() == "" {
		t.Error("GetDestinationTablePath returned empty string")
	}
	if m.GetKnownDestinationsPath() == "" {
		t.Error("GetKnownDestinationsPath returned empty string")
	}
}
