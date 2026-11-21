package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Site.PublicDir != "public" {
		t.Errorf("PublicDir = %q, want public", cfg.Site.PublicDir)
	}
	if !cfg.InPlace() {
		t.Error("InPlace should default to true")
	}
}

func TestLoadParsesValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hugo-reticulum.toml")
	data := `
[reticulum]
node_name = "test-node"
announce_interval_minutes = 30
identity_path = "/tmp/id"

[site]
output_dir = "micron-out"

[markdown]
link_style = "underline"
image_policy = "alt"
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Reticulum.NodeName != "test-node" {
		t.Errorf("NodeName = %q", cfg.Reticulum.NodeName)
	}
	if cfg.AnnounceInterval().Minutes() != 30 {
		t.Errorf("AnnounceInterval = %v", cfg.AnnounceInterval())
	}
	if cfg.InPlace() {
		t.Error("output_dir set should default in_place to false")
	}
	if cfg.OutputPath() != "micron-out" {
		t.Errorf("OutputPath = %q", cfg.OutputPath())
	}
}

func TestNormalizeRejectsBadEnums(t *testing.T) {
	cfg := Default()
	cfg.Markdown.LinkStyle = "blink"
	if err := cfg.Normalize(); err == nil {
		t.Error("expected error for bad link_style")
	}
}

func TestFindWalksUp(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hugo-reticulum.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Find(filepath.Join(dir, "a", "b"))
	want := filepath.Join(dir, "hugo-reticulum.toml")
	if got != want {
		t.Errorf("Find = %q, want %q", got, want)
	}
}
