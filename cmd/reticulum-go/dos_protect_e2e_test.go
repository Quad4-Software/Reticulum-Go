// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/reticulumconfig"
)

func TestE2E_DoSProtectionConfigLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := `[reticulum]
  enable_transport = yes
  enable_sandbox = no
  dos_protection = detect
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := reticulumconfig.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DoSProtection != "detect" {
		t.Fatalf("DoSProtection=%q", cfg.DoSProtection)
	}
	cfg.DoSProtection = "prevent"
	cfg.ConfigPath = path
	if err := reticulumconfig.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "dos_protection = prevent") {
		t.Fatalf("saved config missing prevent: %s", raw)
	}
}
