// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
	"quad4/reticulum-go/pkg/common"
)

// TestLandlockFunctional verifies that Landlock actually blocks access to
// files outside the whitelist. It runs the helper in a subprocess so the
// restriction does not leak into the test runner.
func TestLandlockFunctional(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	// Create directories outside the whitelist. allowedDir is placed inside
	// ~/.reticulum-go (which is whitelisted). blockedDir is placed directly
	// under the home directory (NOT whitelisted).
	allowedDir := filepath.Join(home, ".reticulum-go", "sandbox-test-allowed")
	blockedDir := filepath.Join(home, "sandbox-test-blocked")

	if err := os.MkdirAll(allowedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(blockedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() {
		os.RemoveAll(allowedDir)
		os.RemoveAll(blockedDir)
	}()

	allowedFile := filepath.Join(allowedDir, "allowed.txt")
	blockedFile := filepath.Join(blockedDir, "blocked.txt")

	if err := os.WriteFile(allowedFile, []byte("allowed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blockedFile, []byte("blocked"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestLandlockHelper", "-test.v")
	cmd.Env = append(os.Environ(),
		"SANDBOX_LANDLOCK_TEST=1",
		"SANDBOX_ALLOWED_DIR="+allowedDir,
		"SANDBOX_BLOCKED_DIR="+blockedDir,
	)
	out, err := cmd.CombinedOutput()

	if err != nil {
		if strings.Contains(string(out), "not supported") ||
			strings.Contains(string(out), "operation not permitted") {
			t.Skip("Landlock not available in test environment")
		}
		t.Fatalf("Landlock helper subprocess failed:\n%s", out)
	}

	if !strings.Contains(string(out), "PASS") {
		t.Fatalf("Landlock helper did not report PASS:\n%s", out)
	}
}

// TestLandlockHelper is executed as a subprocess by TestLandlockFunctional.
func TestLandlockHelper(t *testing.T) {
	if os.Getenv("SANDBOX_LANDLOCK_TEST") != "1" {
		t.Skip("helper, run via TestLandlockFunctional")
	}

	allowedDir := os.Getenv("SANDBOX_ALLOWED_DIR")
	blockedDir := os.Getenv("SANDBOX_BLOCKED_DIR")
	if allowedDir == "" || blockedDir == "" {
		t.Fatal("SANDBOX_ALLOWED_DIR and SANDBOX_BLOCKED_DIR must be set")
	}

	allowedFile := filepath.Join(allowedDir, "allowed.txt")
	blockedFile := filepath.Join(blockedDir, "blocked.txt")

	cfg := &common.ReticulumConfig{
		EnableSandbox: true,
		ConfigPath:    filepath.Join(allowedDir, "config"),
	}

	// Landlock requires PR_SET_NO_NEW_PRIVS for unprivileged callers.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		t.Fatalf("PR_SET_NO_NEW_PRIVS failed: %v", err)
	}

	if err := applyLandlock(cfg); err != nil {
		if strings.Contains(err.Error(), "not supported") {
			t.Skip("Landlock not supported:", err)
		}
		t.Fatalf("applyLandlock failed: %v", err)
	}

	data, err := os.ReadFile(allowedFile)
	if err != nil {
		t.Fatalf("expected to read allowed file, got: %v", err)
	}
	if string(data) != "allowed" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	_, err = os.ReadFile(blockedFile)
	if err == nil {
		t.Fatal("expected blocked file read to fail, but it succeeded")
	}

	_, err = os.ReadDir(blockedDir)
	if err == nil {
		t.Fatal("expected blocked directory listing to fail, but it succeeded")
	}
}
