// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"quad4/reticulum-go/pkg/rnsutil"
)

// TestCLIUtilitiesHonorShareInstanceNo locks the rgox/rgopath/rgoprobe interop
// fix: isolated UDP peer configs with share_instance=no must not be overridden
// to attach to a system shared instance (rnsd on :37428).
func TestCLIUtilitiesHonorShareInstanceNo(t *testing.T) {
	forceShare := regexp.MustCompile(`(?m)^\s*cfg\.ShareInstance\s*=\s*true\s*$`)
	for _, name := range []string{"x.go", "path.go", "probe.go"} {
		path := filepath.Join(".", name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if forceShare.Match(body) {
			t.Errorf("%s forces cfg.ShareInstance = true (breaks share_instance=no interop configs)", name)
		}
	}
}

func TestLoadConfigDirPreservesShareInstanceNo(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	content := "[reticulum]\n" +
		"enable_transport = yes\n" +
		"share_instance = no\n" +
		"\n" +
		"[interfaces]\n" +
		"  [[UDP]]\n" +
		"    type = UDPInterface\n" +
		"    enabled = yes\n" +
		"    listen_ip = 127.0.0.1\n" +
		"    listen_port = 0\n" +
		"    target_host = 127.0.0.1\n" +
		"    target_port = 1\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := rnsutil.LoadConfigDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ShareInstance {
		t.Fatal("expected ShareInstance=false from share_instance=no")
	}
}
