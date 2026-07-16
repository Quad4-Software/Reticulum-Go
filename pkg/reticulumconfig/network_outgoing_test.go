// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_NetworkIdentityAndOutgoing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	body := `[reticulum]
  enable_transport = yes
  share_instance = no
  network_identity = ~/.reticulum-go/storage/identities/mynet

[interfaces]
  [[Listen Only]]
    type = UDPInterface
    enabled = yes
    address = 0.0.0.0
    port = 4242
    target_address = 127.0.0.1:4243
    outgoing = no
  [[Selected Alias]]
    type = UDPInterface
    enabled = yes
    address = 0.0.0.0
    port = 4244
    target_address = 127.0.0.1:4245
    selected_outgoing = false
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.NetworkIdentityPath != "~/.reticulum-go/storage/identities/mynet" {
		t.Fatalf("NetworkIdentityPath: got %q", cfg.NetworkIdentityPath)
	}
	lo := cfg.Interfaces["Listen Only"]
	if lo == nil || !lo.OutgoingSet || lo.Outgoing {
		t.Fatalf("Listen Only outgoing: %+v", lo)
	}
	sa := cfg.Interfaces["Selected Alias"]
	if sa == nil || !sa.OutgoingSet || sa.Outgoing {
		t.Fatalf("Selected Alias outgoing: %+v", sa)
	}
}
