// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadServerConfigAliases(t *testing.T) {
	dir := writeConfig(t, `[rngit]
node_name = test

[aliases]
admin = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
all = bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
bad = nothex
short = aaaa

[repositories]
public = ./repos
`)
	cfg, err := LoadServerConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IdentityAliases["admin"] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("alias: %v", cfg.IdentityAliases)
	}
	if _, ok := cfg.IdentityAliases["all"]; ok {
		t.Fatal("reserved target must not be accepted as alias")
	}
	if _, ok := cfg.IdentityAliases["bad"]; ok {
		t.Fatal("invalid hex alias accepted")
	}
	if _, ok := cfg.IdentityAliases["short"]; ok {
		t.Fatal("short hash alias accepted")
	}
}

func TestLoadServerConfigBlockedIdentities(t *testing.T) {
	dir := writeConfig(t, `[rngit]
blocked_identities = d7db22f63b453c23bb0688dde565b7c1, crawler, badvalue

[aliases]
crawler = d31aeea49873006f13b3415520666a4e

`)
	cfg, err := LoadServerConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.BlockedIdentities["d7db22f63b453c23bb0688dde565b7c1"] {
		t.Fatal("null identity not blocked")
	}
	if !cfg.BlockedIdentities["d31aeea49873006f13b3415520666a4e"] {
		t.Fatal("alias-resolved identity not blocked")
	}
	if len(cfg.BlockedIdentities) != 2 {
		t.Fatalf("blocked: %v", cfg.BlockedIdentities)
	}
}

func TestLoadServerConfigMediaConversion(t *testing.T) {
	// Default is enabled.
	dir := writeConfig(t, "[rngit]\nnode_name = t\n")
	cfg, err := LoadServerConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.MediaConversion {
		t.Fatal("media conversion should default to enabled")
	}

	dir = writeConfig(t, "[pages]\nmedia_conversion = no\n")
	cfg, err = LoadServerConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MediaConversion {
		t.Fatal("media_conversion = no not applied")
	}
}

func TestNullIdentHash(t *testing.T) {
	// The Python null identity is an all-zero 64-byte keypair; its truncated
	// identity hash is fixed and must match RNS for no_ident behavior.
	if nullIdentHash != "d7db22f63b453c23bb0688dde565b7c1" {
		t.Fatalf("null ident: %s", nullIdentHash)
	}
}
