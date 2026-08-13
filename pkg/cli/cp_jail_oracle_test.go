// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression: rncp -jail must not follow a symlink placed inside the jail
// that points outside it.
func TestOracleCPJailRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	jail, err := filepath.Abs(filepath.Join(root, "jail"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(jail, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(jail, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	if got, ok := resolveFetchPath(link, jail); ok {
		t.Fatalf("jail followed symlink escape: %q -> %q", link, got)
	}
}

// Legitimate requests for real files inside the jail must still resolve.
func TestOracleCPJailAllowsRealFile(t *testing.T) {
	root := t.TempDir()
	jail, err := filepath.Abs(filepath.Join(root, "jail"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(jail, 0o750); err != nil {
		t.Fatal(err)
	}
	realFile := filepath.Join(jail, "data.txt")
	if err := os.WriteFile(realFile, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := resolveFetchPath(realFile, jail)
	if !ok {
		t.Fatal("jail rejected a legitimate file inside jail dir")
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("resolved path not readable: %v", err)
	}
}
