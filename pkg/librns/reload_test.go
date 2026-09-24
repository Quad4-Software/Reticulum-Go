// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"os"
	"path/filepath"
	"testing"
)

func writeUDPConfig(t *testing.T, path string, enabled bool) {
	t.Helper()
	en := "yes"
	if !enabled {
		en = "no"
	}
	body := `[reticulum]
  enable_transport = yes
  share_instance = no

[interfaces]
  [[udpreload]]
    type = UDPInterface
    enabled = ` + en + `
    address = 127.0.0.1:0
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNodeReloadConfigRequiresPath(t *testing.T) {
	node := mustCreateNode(t)
	if code := NodeReloadConfig(node); code != ErrInvalidArg {
		t.Fatalf("empty path reload: got %d want %d err=%q", code, ErrInvalidArg, LastError())
	}
}

func TestNodeReloadConfigDisableReenableUDP(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	path := filepath.Join(t.TempDir(), "config")
	writeUDPConfig(t, path, true)

	node, code := NodeCreate(path)
	if code != OK || node == 0 {
		t.Fatalf("NodeCreate: code=%d err=%q", code, LastError())
	}
	t.Cleanup(func() { _ = NodeDestroy(node) })

	if code := NodeStart(node); code != OK {
		t.Fatalf("NodeStart: %d %q", code, LastError())
	}

	entries, code := NodeInterfaces(node)
	if code != OK {
		t.Fatalf("NodeInterfaces: %d %q", code, LastError())
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 iface after start, got %d", len(entries))
	}

	writeUDPConfig(t, path, false)
	if code := NodeReloadConfig(node); code != OK {
		t.Fatalf("reload disable: %d %q", code, LastError())
	}
	entries, code = NodeInterfaces(node)
	if code != OK {
		t.Fatal(code, LastError())
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 ifaces after disable, got %d", len(entries))
	}

	writeUDPConfig(t, path, true)
	if code := NodeReloadConfig(node); code != OK {
		t.Fatalf("reload enable: %d %q", code, LastError())
	}
	entries, code = NodeInterfaces(node)
	if code != OK {
		t.Fatal(code, LastError())
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 iface after reenable, got %d", len(entries))
	}
}

func TestNodeReloadConfigMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	writeUDPConfig(t, path, true)

	node, code := NodeCreate(path)
	if code != OK || node == 0 {
		t.Fatalf("NodeCreate: code=%d err=%q", code, LastError())
	}
	t.Cleanup(func() { _ = NodeDestroy(node) })

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if code := NodeReloadConfig(node); code != ErrIO {
		t.Fatalf("missing config reload: got %d want %d err=%q", code, ErrIO, LastError())
	}
}
