// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package pageserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression: path jail must reject sibling-prefix escapes (pages vs pages_secret).
func TestOraclePageJailPrefixEscape(t *testing.T) {
	root := t.TempDir()
	pages := filepath.Join(root, "pages")
	sibling := filepath.Join(root, "pages_secret")
	if err := os.MkdirAll(pages, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0o750); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(sibling, "leak.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	joined := filepath.Join(pages, "../pages_secret/leak.txt")
	if got, allowed := resolveJailedPath(pages, joined); allowed {
		t.Fatalf("jail accepted prefix escape %q -> %q", joined, got)
	}
}

// Regression: a symlink under pages must not resolve outside the jail.
func TestOraclePageJailSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	pages := filepath.Join(root, "pages")
	outside := filepath.Join(root, "outside.txt")
	if err := os.MkdirAll(pages, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(pages, "escape.mu")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	if got, allowed := resolveJailedPath(pages, link); allowed {
		t.Fatalf("jail followed symlink outside pages dir: %q -> %q", link, got)
	}
}

// Regression: a symlinked ancestor directory must not smuggle a nonexistent
// leaf path outside the jail either.
func TestOraclePageJailSymlinkedAncestorEscape(t *testing.T) {
	root := t.TempDir()
	pages := filepath.Join(root, "pages")
	outsideDir := filepath.Join(root, "outside_dir")
	if err := os.MkdirAll(pages, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o750); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outsideDir, "leak.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(pages, "linkdir")
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		t.Fatal(err)
	}

	joined := filepath.Join(pages, "linkdir", "leak.txt")
	if got, allowed := resolveJailedPath(pages, joined); allowed {
		t.Fatalf("jail followed symlinked ancestor outside pages dir: %q -> %q", joined, got)
	}
}

// Legitimate requests for real files inside the jail must still resolve.
func TestOraclePageJailAllowsRealFile(t *testing.T) {
	root := t.TempDir()
	pages := filepath.Join(root, "pages")
	if err := os.MkdirAll(pages, 0o750); err != nil {
		t.Fatal(err)
	}
	realFile := filepath.Join(pages, "index.mu")
	if err := os.WriteFile(realFile, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, allowed := resolveJailedPath(pages, realFile)
	if !allowed {
		t.Fatal("jail rejected a legitimate file inside pages dir")
	}
	resolvedPages, err := filepath.EvalSymlinks(pages)
	if err != nil {
		resolvedPages = pages
	}
	if !strings.HasPrefix(got, resolvedPages+string(os.PathSeparator)) {
		t.Fatalf("resolved path %q escaped pages dir %q", got, resolvedPages)
	}
}

// A legitimate 404 for a path under a real directory must still resolve
// (not be misreported as a jail violation).
func TestOraclePageJailAllowsMissingLeafUnderRealDir(t *testing.T) {
	root := t.TempDir()
	pages := filepath.Join(root, "pages")
	if err := os.MkdirAll(pages, 0o750); err != nil {
		t.Fatal(err)
	}

	joined := filepath.Join(pages, "missing.mu")
	if _, allowed := resolveJailedPath(pages, joined); !allowed {
		t.Fatal("jail rejected a missing leaf under a real, non-symlinked directory")
	}
}
