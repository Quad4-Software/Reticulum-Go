// Package config loads hugo-reticulum.toml settings.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the hugo-reticulum.toml file format.
type Config struct {
	// Hugo is the path to the hugo binary or additional hugo arguments.
	Hugo struct {
		Path string   `toml:"path"`
		Args []string `toml:"args"`
	} `toml:"hugo"`

	// Site controls the build output location.
	Site struct {
		// SourceDir is the Hugo site directory. Empty means the current
		// directory or the directory containing the config file.
		SourceDir string `toml:"source_dir"`
		// PublicDir is the Hugo publish directory inside SourceDir.
		// Default "public".
		PublicDir string `toml:"public_dir"`
		// OutputDir is where final Micron files are written. Empty means
		// in-place inside PublicDir.
		OutputDir string `toml:"output_dir"`
		// StaticDir is the directory served under /file/. Default "static".
		StaticDir string `toml:"static_dir"`
		// InPlace rewrites .mu files inside PublicDir instead of copying
		// to OutputDir. Default true when OutputDir is empty.
		InPlace *bool `toml:"in_place"`
	} `toml:"site"`

	// Reticulum controls the network node.
	Reticulum struct {
		// ConfigPath is the Reticulum-Go config file. Empty uses the
		// default ~/.reticulum-go/config.
		ConfigPath string `toml:"config_path"`
		// IdentityPath overrides the identity file location.
		IdentityPath string `toml:"identity_path"`
		// NodeName is the announce display name.
		NodeName string `toml:"node_name"`
		// AnnounceIntervalMinutes is the periodic announce period.
		// Zero announces once at startup.
		AnnounceIntervalMinutes int `toml:"announce_interval_minutes"`
		// PagesDir and FilesDir override the served directories. Empty
		// values derive from the site layout.
		PagesDir string `toml:"pages_dir"`
		FilesDir string `toml:"files_dir"`
		// PageRefreshSeconds and FileRefreshSeconds rescan the served
		// directories on an interval. Zero disables rescanning.
		PageRefreshSeconds int `toml:"page_refresh_seconds"`
		FileRefreshSeconds int `toml:"file_refresh_seconds"`
		// DisablePageStats turns off the built-in page view counter.
		DisablePageStats bool `toml:"disable_page_stats"`
		// NodeHash prefixes links as "<hash>:/page/...". Usually left
		// empty so links stay node relative.
		NodeHash string `toml:"node_hash"`
	} `toml:"reticulum"`

	// Markdown controls the Markdown to Micron conversion.
	Markdown struct {
		MaxWidth    int    `toml:"max_width"`
		LinkStyle   string `toml:"link_style"`
		LinkColor   string `toml:"link_color"`
		ImagePolicy string `toml:"image_policy"`
		TableAlign  string `toml:"table_align"`
	} `toml:"markdown"`

	// Log is the log level name: debug, info, warn, or error.
	Log string `toml:"log"`
}

// Default returns a Config with site defaults filled in.
func Default() *Config {
	c := &Config{}
	c.Site.PublicDir = "public"
	c.Site.StaticDir = "static"
	c.Reticulum.AnnounceIntervalMinutes = 360
	c.Markdown.MaxWidth = 100
	c.Markdown.LinkStyle = "both"
	c.Markdown.ImagePolicy = "link"
	c.Markdown.TableAlign = "center"
	c.Log = "info"
	return c
}

// Load reads path when it exists, otherwise returns Default. A missing file
// is not an error so serve works without configuration.
func Load(path string) (*Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	} else if err != nil {
		return nil, fmt.Errorf("stat config %q: %w", path, err)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- operator supplied path
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	if _, err := toml.Decode(string(data), cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	cfg.applyDefaults()
	return cfg, nil
}

// Find locates hugo-reticulum.toml by walking up from dir.
func Find(dir string) string {
	for {
		candidate := filepath.Join(dir, "hugo-reticulum.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func (c *Config) applyDefaults() {
	d := Default()
	if c.Site.PublicDir == "" {
		c.Site.PublicDir = d.Site.PublicDir
	}
	if c.Site.StaticDir == "" {
		c.Site.StaticDir = d.Site.StaticDir
	}
	if c.Site.InPlace == nil {
		c.Site.InPlace = d.Site.InPlace
	}
	if c.Markdown.MaxWidth == 0 {
		c.Markdown.MaxWidth = d.Markdown.MaxWidth
	}
	if c.Markdown.LinkStyle == "" {
		c.Markdown.LinkStyle = d.Markdown.LinkStyle
	}
	if c.Markdown.ImagePolicy == "" {
		c.Markdown.ImagePolicy = d.Markdown.ImagePolicy
	}
	if c.Markdown.TableAlign == "" {
		c.Markdown.TableAlign = d.Markdown.TableAlign
	}
}

// SourceDir resolves the effective Hugo site directory.
func (c *Config) SourceDir() string {
	if c.Site.SourceDir != "" {
		return c.Site.SourceDir
	}
	return "."
}

// PublicPath resolves the absolute publish directory.
func (c *Config) PublicPath() string {
	return filepath.Join(c.SourceDir(), c.Site.PublicDir)
}

// StaticPath resolves the absolute static directory.
func (c *Config) StaticPath() string {
	return filepath.Join(c.SourceDir(), c.Site.StaticDir)
}

// OutputPath resolves the directory that receives final Micron files.
// Relative output_dir values resolve against the site directory. When
// in-place, it equals PublicPath.
func (c *Config) OutputPath() string {
	if c.Site.OutputDir == "" {
		return c.PublicPath()
	}
	if filepath.IsAbs(c.Site.OutputDir) {
		return c.Site.OutputDir
	}
	return filepath.Join(c.SourceDir(), c.Site.OutputDir)
}

// InPlace reports whether output replaces files inside the publish dir.
// When in_place is unset it defaults to true unless output_dir is set.
func (c *Config) InPlace() bool {
	if c.Site.InPlace != nil {
		return *c.Site.InPlace
	}
	return c.Site.OutputDir == ""
}

// AnnounceInterval returns the announce period as a duration.
func (c *Config) AnnounceInterval() time.Duration {
	return time.Duration(c.Reticulum.AnnounceIntervalMinutes) * time.Minute
}

// Normalize trims strings and validates enum-like fields.
func (c *Config) Normalize() error {
	c.Reticulum.NodeName = strings.TrimSpace(c.Reticulum.NodeName)
	c.Reticulum.NodeHash = strings.TrimSpace(c.Reticulum.NodeHash)
	switch strings.ToLower(c.Markdown.LinkStyle) {
	case "", "plain", "underline", "bold", "both", "color":
	default:
		return fmt.Errorf("markdown.link_style %q is invalid, want plain, underline, bold, both, or color", c.Markdown.LinkStyle)
	}
	switch strings.ToLower(c.Markdown.ImagePolicy) {
	case "", "link", "alt", "skip":
	default:
		return fmt.Errorf("markdown.image_policy %q is invalid, want link, alt, or skip", c.Markdown.ImagePolicy)
	}
	return nil
}
