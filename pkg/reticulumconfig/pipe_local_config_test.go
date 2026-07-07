// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigPipeInterfaceKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	cfgText := strings.Join([]string{
		"[[Pipe]]",
		"type = PipeInterface",
		"enabled = yes",
		`command = echo hello`,
		"respawn_delay = 10",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(cfgText), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	ic, ok := cfg.Interfaces["Pipe"]
	if !ok {
		t.Fatal("missing Pipe interface")
	}
	if ic.Type != "PipeInterface" {
		t.Fatalf("type = %q", ic.Type)
	}
	if ic.Command != "echo hello" {
		t.Fatalf("command = %q", ic.Command)
	}
	if ic.RespawnDelay != 10 {
		t.Fatalf("respawn_delay = %d", ic.RespawnDelay)
	}
}

func TestLoadConfigLocalInterfaceKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	cfgText := strings.Join([]string{
		"[[Local Client]]",
		"type = LocalInterface",
		"enabled = yes",
		"port = 43777",
		"shared_instance_type = unix",
		"instance_name = myinst",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(cfgText), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	ic, ok := cfg.Interfaces["Local Client"]
	if !ok {
		t.Fatal("missing Local Client interface")
	}
	if ic.Port != 43777 {
		t.Fatalf("port = %d", ic.Port)
	}
	if ic.SharedInstanceType != "unix" {
		t.Fatalf("shared_instance_type = %q", ic.SharedInstanceType)
	}
	if ic.InstanceName != "myinst" {
		t.Fatalf("instance_name = %q", ic.InstanceName)
	}
}
