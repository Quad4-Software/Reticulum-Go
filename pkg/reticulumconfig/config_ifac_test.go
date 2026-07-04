// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigIFACAndSharedInstance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := `
[reticulum]
  share_instance = yes
  shared_instance_port = 37428
  instance_control_port = 37429
  shared_instance_type = tcp
  instance_name = node1

[interfaces]

  [[Test UDP]]
    type = UDPInterface
    enabled = yes
    network_name = mynet
    passphrase = secret
    ifac_size = 128
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.ShareInstance {
		t.Fatal("expected share_instance")
	}
	if cfg.SharedInstanceType != "tcp" {
		t.Fatalf("shared_instance_type = %q", cfg.SharedInstanceType)
	}
	if cfg.InstanceName != "node1" {
		t.Fatalf("instance_name = %q", cfg.InstanceName)
	}
	ic := cfg.Interfaces["Test UDP"]
	if ic == nil {
		t.Fatal("missing interface")
	}
	if ic.NetworkName != "mynet" || ic.Passphrase != "secret" {
		t.Fatalf("network creds: name=%q pass=%q", ic.NetworkName, ic.Passphrase)
	}
	if ic.IFACSize != 16 {
		t.Fatalf("IFACSize = %d, want 16", ic.IFACSize)
	}
}

func TestSetIFACSizeParser(t *testing.T) {
	var size int
	setIFACSize("128", &size)
	if size != 16 {
		t.Fatalf("128 bits => %d bytes, want 16", size)
	}
	size = 0
	setIFACSize("8", &size)
	if size != 1 {
		t.Fatalf("8 bits => %d bytes, want 1", size)
	}
}
