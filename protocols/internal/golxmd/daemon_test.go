// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

func TestFirstRunCreatesFiles(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, "config")
	identPath := filepath.Join(home, "identity")
	storageDir := filepath.Join(home, "storage")
	messagesDir := filepath.Join(storageDir, "messages")
	rnsDir := filepath.Join(home, "rns")

	created, err := FirstRun(home, cfgPath, identPath, storageDir, messagesDir, rnsDir)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected first run to create files")
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := os.Stat(identPath); err != nil {
		t.Fatalf("identity: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rnsDir, "config")); err != nil {
		t.Fatalf("rns config: %v", err)
	}

	created2, err := FirstRun(home, cfgPath, identPath, storageDir, messagesDir, rnsDir)
	if err != nil {
		t.Fatal(err)
	}
	if created2 {
		t.Fatal("second run should not recreate")
	}
}

func TestConfigLoadFromFirstRun(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, "config")
	identPath := filepath.Join(home, "identity")
	storageDir := filepath.Join(home, "storage")
	messagesDir := filepath.Join(storageDir, "messages")
	if _, err := FirstRun(home, cfgPath, identPath, storageDir, messagesDir, filepath.Join(home, "rns")); err != nil {
		t.Fatal(err)
	}
	cfg, err := lxmf.LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LXMF.DisplayName == "" {
		t.Fatal("expected display name in default config")
	}
}

func TestDefaultPaths(t *testing.T) {
	t.Setenv("GOLXMD_HOME", "")
	home := DefaultHome()
	if home == "" {
		t.Fatal("empty home")
	}
	if filepath.Base(DefaultConfigPath()) != "config" {
		t.Fatal("config path")
	}
}

func TestPrettyHexDestinationHash(t *testing.T) {
	raw := "a3a523f48208a950b026ccc0d8b702ac"
	if got := prettyHex(raw); got != raw {
		t.Fatalf("prettyHex(%q) = %q", raw, got)
	}
	dotted := "a3.a5.23.f4.82.08.a9.50.b0.26.cc.c0.d8.b7.02.ac"
	if got := prettyHex(dotted); got != raw {
		t.Fatalf("prettyHex(dotted) = %q want %q", got, raw)
	}
	upper := strings.ToUpper(raw)
	if got := prettyHex(upper); got != raw {
		t.Fatalf("prettyHex(upper) = %q want %q", got, raw)
	}
}

func TestDecodeDestHashColonSeparated(t *testing.T) {
	raw := "a3a523f48208a950b026ccc0d8b702ac"
	colon := "a3:a5:23:f4:82:08:a9:50:b0:26:cc:c0:d8:b7:02:ac"
	got, err := decodeDestHash(colon)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != raw {
		t.Fatalf("got %x want %s", got, raw)
	}
}
