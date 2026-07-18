// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBughuntSandboxTypoKeepsDefault ensures unrecognized boolean spellings
// do not silently disable security defaults.
func TestBughuntSandboxTypoKeepsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	content := `[reticulum]
enable_sandbox = yeah
enable_seccomp = enable
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.EnableSandbox {
		t.Fatal("typo disabled EnableSandbox")
	}
	if !cfg.EnableSeccomp {
		t.Fatal("typo disabled EnableSeccomp")
	}
}
