// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDestFromArgs(t *testing.T) {
	got := destFromArgs("", []string{"9b246eead5744d3b77030c88f6e54c98"})
	if got != "9b246eead5744d3b77030c88f6e54c98" {
		t.Fatalf("positional %q", got)
	}
	got = destFromArgs("aabbcc", nil)
	if got != "aabbcc" {
		t.Fatalf("flag %q", got)
	}
	got = destFromArgs("old", []string{"9B24 6EEA D574 4D3B 7703 0C88 F6E5 4C98"})
	if got != "9b246eead5744d3b77030c88f6e54c98" {
		t.Fatalf("spaces %q", got)
	}
	if destFromArgs("", nil) != "" {
		t.Fatal("empty")
	}
}

func TestNormalizeHash(t *testing.T) {
	got := normalizeHash("<9B24:6eea-d574 4d3b>")
	if got != "9b246eead5744d3b" {
		t.Fatalf("%q", got)
	}
}

func TestLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rgesp.conf")
	body := "# comment\nidentity = /tmp/id\nprofile = hq\nserver = true\nlisten-port = 9000\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	vals, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if vals["identity"] != "/tmp/id" || vals["profile"] != "hq" || vals["server"] != "true" {
		t.Fatalf("parsed %+v", vals)
	}
	if vals["listen-port"] != "9000" {
		t.Fatalf("listen-port %q", vals["listen-port"])
	}
}

func TestLoadConfigFileRejectsBadLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.conf")
	if err := os.WriteFile(path, []byte("not a pair\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfigFile(path); err == nil {
		t.Fatal("expected error")
	}
}
