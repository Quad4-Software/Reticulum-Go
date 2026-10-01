// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"path/filepath"
	"testing"
)

func TestLoadConfigUDPKeepalive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	writeFile(t, path, `[interfaces]
  [[u]]
    type = UDPInterface
    enabled = yes
    listen_ip = 127.0.0.1
    listen_port = 4242
    forward_ip = 127.0.0.1
    forward_port = 4243
    keepalive = 25
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	iface := cfg.Interfaces["u"]
	if iface == nil {
		t.Fatal("u interface missing")
	}
	if iface.KeepaliveSec != 25 {
		t.Fatalf("KeepaliveSec = %d, want 25", iface.KeepaliveSec)
	}
}

func TestLoadConfigPersistentKeepaliveAlias(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	writeFile(t, path, `[interfaces]
  [[u]]
    type = UDPInterface
    persistent_keepalive = 30
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.Interfaces["u"].KeepaliveSec; got != 30 {
		t.Fatalf("KeepaliveSec = %d, want 30", got)
	}
}

// TestLoadConfigKeepaliveUnset verifies the default stays disabled so no
// on-wire behavior changes unless the operator opts in.
func TestLoadConfigKeepaliveUnset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	writeFile(t, path, `[interfaces]
  [[u]]
    type = UDPInterface
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.Interfaces["u"].KeepaliveSec; got != 0 {
		t.Fatalf("KeepaliveSec default = %d, want 0", got)
	}
}
