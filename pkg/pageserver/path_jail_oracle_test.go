// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package pageserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// servePageJailCheck mirrors the Clean+HasPrefix check in servePage/serveFile.
func servePageJailCheck(pagesPath, path string) (filePath string, allowed bool) {
	var joined string
	if after, ok := strings.CutPrefix(path, "/page/"); ok {
		joined = filepath.Join(pagesPath, after)
	} else {
		joined = filepath.Join(pagesPath, path)
	}
	filePath = filepath.Clean(joined)
	pagesDir := filepath.Clean(pagesPath)
	return filePath, strings.HasPrefix(filePath, pagesDir)
}

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

	req := "/page/../pages_secret/leak.txt"
	got, allowed := servePageJailCheck(pages, req)
	if allowed {
		t.Fatalf("jail accepted prefix escape %q -> %q", req, got)
	}
}

// Regression: symlink under pages must not open outside the jail.
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

	got, allowed := servePageJailCheck(pages, "/page/escape.mu")
	if !allowed {
		return
	}
	resolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	pagesReal, err := filepath.EvalSymlinks(pages)
	if err != nil {
		pagesReal = pages
	}
	if resolved == outside || !strings.HasPrefix(resolved, pagesReal+string(os.PathSeparator)) {
		t.Fatalf("jail allowed symlink escape: link=%q resolved=%q outside=%q", got, resolved, outside)
	}
}
