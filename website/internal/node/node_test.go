package node

import (
	"testing"
	"time"
)

func TestBuildOptions(t *testing.T) {
	opts := buildOptions(Params{
		IdentityPath:        "/tmp/id",
		NodeName:            "my node!",
		AnnounceInterval:    90 * time.Minute,
		PagesDir:            "/srv/pages",
		FilesDir:            "/srv/files",
		PageRefreshInterval: 5 * time.Second,
		FileRefreshInterval: 7 * time.Second,
		DisablePageStats:    true,
	})
	if opts.PagesDir != "/srv/pages" {
		t.Errorf("PagesDir = %q", opts.PagesDir)
	}
	if opts.FilesDir != "/srv/files" {
		t.Errorf("FilesDir = %q", opts.FilesDir)
	}
	if opts.AnnounceIntervalMinutes != 90 {
		t.Errorf("AnnounceIntervalMinutes = %d", opts.AnnounceIntervalMinutes)
	}
	if opts.IdentityFileOverride != "/tmp/id" {
		t.Errorf("IdentityFileOverride = %q", opts.IdentityFileOverride)
	}
	if !opts.DisablePageStats {
		t.Error("DisablePageStats not propagated")
	}
	if opts.PageRefreshInterval != 5*time.Second {
		t.Errorf("PageRefreshInterval = %v", opts.PageRefreshInterval)
	}
	if opts.FileRefreshInterval != 7*time.Second {
		t.Errorf("FileRefreshInterval = %v", opts.FileRefreshInterval)
	}
}

func TestBuildOptionsDefaults(t *testing.T) {
	opts := buildOptions(Params{})
	if opts.NodeDisplayName == "" {
		t.Error("NodeDisplayName should default to a non-empty name")
	}
	if opts.AnnounceIntervalMinutes != 0 {
		t.Errorf("AnnounceIntervalMinutes = %d, want 0", opts.AnnounceIntervalMinutes)
	}
}
