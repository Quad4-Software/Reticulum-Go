// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPortRange(t *testing.T) {
	if _, err := parsePort("listen-port", "0"); err == nil {
		t.Fatal("port 0")
	}
	if _, err := parsePort("listen-port", "65536"); err == nil {
		t.Fatal("port 65536")
	}
	if _, err := parsePort("listen-port", "-1"); err == nil {
		t.Fatal("negative")
	}
	n, err := parsePort("listen-port", "4242")
	if err != nil || n != 4242 {
		t.Fatalf("%d %v", n, err)
	}
}

func TestConfigEmptyKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.conf")
	if err := os.WriteFile(path, []byte("= value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfigFile(path); err == nil {
		t.Fatal("empty key")
	}
}

func TestConfigIgnoresCommentsAndBlank(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.conf")
	body := "\n# hi\nprofile = ll\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	vals, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if vals["profile"] != "ll" {
		t.Fatalf("%+v", vals)
	}
}

func TestApplyConfigSkipsSetFlags(t *testing.T) {
	old := *profileName
	t.Cleanup(func() { *profileName = old })
	*profileName = "mq"
	if err := applyConfigMap(map[string]string{"profile": "hq"}, map[string]bool{"profile": true}); err != nil {
		t.Fatal(err)
	}
	if *profileName != "mq" {
		t.Fatalf("flag should win, got %s", *profileName)
	}
}

func TestApplyConfigFillsUnset(t *testing.T) {
	old := *profileName
	t.Cleanup(func() { *profileName = old })
	*profileName = "mq"
	if err := applyConfigMap(map[string]string{"profile": "hq"}, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	if *profileName != "hq" {
		t.Fatalf("config should fill, got %s", *profileName)
	}
}

func FuzzParsePort(f *testing.F) {
	f.Add("4242")
	f.Add("0")
	f.Add("65535")
	f.Add("65536")
	f.Add("-3")
	f.Add("x")
	f.Fuzz(func(t *testing.T, s string) {
		n, err := parsePort("p", s)
		if err == nil && (n < 1 || n > 65535) {
			t.Fatalf("accepted out of range %d from %q", n, s)
		}
	})
}

func TestApplyConfigBadBool(t *testing.T) {
	if err := applyConfigMap(map[string]string{"server": "maybe"}, map[string]bool{}); err == nil {
		t.Fatal("bad bool")
	}
}

func TestApplyConfigRejectsSandbox(t *testing.T) {
	keys := []string{"sandbox", "no-sandbox", "nosandbox", "seccomp", "landlock", "disable-sandbox"}
	for _, k := range keys {
		if err := applyConfigMap(map[string]string{k: "off"}, map[string]bool{}); err == nil {
			t.Fatalf("accepted %s", k)
		}
	}
}
