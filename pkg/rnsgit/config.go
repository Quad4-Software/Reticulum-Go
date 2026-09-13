// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"quad4/reticulum-go/pkg/identity"
)

// ClientConfig holds git-remote-rns client settings.
type ClientConfig struct {
	ConfigDir    string
	RefBatchSize int
	DestAliases  map[string]string
	LogLevel     int
	IdentityPath string
	RNSConfigDir string
}

// ServerConfig holds rngit node settings.
type ServerConfig struct {
	ConfigDir         string
	NodeName          string
	AnnounceInterval  int
	RecordStats       bool
	MirrorIntervalHrs int
	RepositoryGroups  map[string]string
	AccessRules       map[string]string
	ServeNomadNet     bool
	UnicodeIcons      bool
	MediaConversion   bool
	BlockedIdentities map[string]bool
	IdentityAliases   map[string]string
	RNSConfigDir      string
	IdentityPath      string
	LogLevel          int
}

// DefaultClientConfigDir returns ~/.rngit or /etc/rngit.
func DefaultClientConfigDir() string {
	home := os.Getenv("HOME")
	if home != "" {
		legacy := filepath.Join(home, ".config", "rngit", "config")
		if st, err := os.Stat(legacy); err == nil && !st.IsDir() { // #nosec G703 -- legacy config probe
			return filepath.Join(home, ".rngit", "reticulum")
		}
		return filepath.Join(home, ".rngit")
	}
	return "/etc/rngit"
}

// DefaultServerConfigDir returns the rngit server config directory.
func DefaultServerConfigDir() string {
	if st, err := os.Stat("/etc/rngit/config"); err == nil && !st.IsDir() {
		return "/etc/rngit"
	}
	return DefaultClientConfigDir()
}

// LoadClientConfig reads ~/.rngit/client_config.
func LoadClientConfig(dir string) (*ClientConfig, error) {
	if dir == "" {
		dir = os.Getenv("RNGIT_CONFIG")
	}
	if dir == "" {
		dir = DefaultClientConfigDir()
	}
	cfg := &ClientConfig{
		ConfigDir:    dir,
		RefBatchSize: DefaultRefBatchSize,
		DestAliases:  map[string]string{},
		LogLevel:     4,
		IdentityPath: filepath.Join(dir, "client_identity"),
	}
	path := filepath.Join(dir, "client_config")
	sections, err := parseINI(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if v, ok := sections["client"]["ref_batch_size"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			cfg.RefBatchSize = clampInt(n, 1, 1024)
		}
	}
	if v, ok := sections["logging"]["loglevel"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			cfg.LogLevel = clampInt(n, 0, 7)
		}
	}
	for k, v := range sections["aliases"] {
		cfg.DestAliases[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return cfg, nil
}

// LoadServerConfig reads rngit server config.
func LoadServerConfig(dir string) (*ServerConfig, error) {
	if dir == "" {
		dir = os.Getenv("RNGIT_CONFIG")
	}
	if dir == "" {
		dir = DefaultServerConfigDir()
	}
	cfg := &ServerConfig{
		ConfigDir:         dir,
		NodeName:          "Reticulum Git Node",
		AnnounceInterval:  360,
		MirrorIntervalHrs: 24,
		RepositoryGroups:  map[string]string{},
		AccessRules:       map[string]string{},
		MediaConversion:   true,
		BlockedIdentities: map[string]bool{},
		IdentityAliases:   map[string]string{},
		IdentityPath:      filepath.Join(dir, "server_identity"),
		LogLevel:          4,
	}
	path := filepath.Join(dir, "config")
	sections, err := parseINI(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if v, ok := sections["rngit"]["node_name"]; ok {
		cfg.NodeName = strings.TrimSpace(v)
	}
	if v, ok := sections["rngit"]["announce_interval"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			cfg.AnnounceInterval = n
		}
	}
	if v, ok := sections["rngit"]["record_stats"]; ok {
		cfg.RecordStats = strings.EqualFold(strings.TrimSpace(v), "yes")
	}
	if v, ok := sections["rngit"]["mirror_interval"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			cfg.MirrorIntervalHrs = n
		}
	}
	for k, v := range sections["aliases"] {
		alias := strings.TrimSpace(k)
		switch strings.ToLower(alias) {
		case "n", "none", "nobody", "a", "all", "everyone":
			continue
		}
		if _, dup := cfg.IdentityAliases[alias]; dup {
			continue
		}
		hexHash := strings.ToLower(strings.TrimSpace(v))
		if len(hexHash) != 32 {
			continue
		}
		if _, err := hex.DecodeString(hexHash); err != nil {
			continue
		}
		cfg.IdentityAliases[alias] = hexHash
	}
	if v, ok := sections["rngit"]["blocked_identities"]; ok {
		for _, entry := range splitListValue(v) {
			resolved := cfg.resolveIdentityAliasLocked(entry)
			if len(resolved) == 32 {
				if _, err := hex.DecodeString(resolved); err == nil {
					cfg.BlockedIdentities[strings.ToLower(resolved)] = true
				}
			}
		}
	}
	for k, v := range sections["repositories"] {
		cfg.RepositoryGroups[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	for k, v := range sections["access"] {
		cfg.AccessRules[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if v, ok := sections["pages"]["serve_nomadnet"]; ok {
		cfg.ServeNomadNet = strings.EqualFold(strings.TrimSpace(v), "yes")
	}
	if v, ok := sections["pages"]["unicode_icons"]; ok {
		cfg.ServeNomadNet = cfg.ServeNomadNet || strings.EqualFold(strings.TrimSpace(v), "yes")
		cfg.UnicodeIcons = strings.EqualFold(strings.TrimSpace(v), "yes")
	}
	if v, ok := sections["pages"]["media_conversion"]; ok {
		cfg.MediaConversion = strings.EqualFold(strings.TrimSpace(v), "yes")
	}
	if v, ok := sections["logging"]["loglevel"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			cfg.LogLevel = clampInt(n, 0, 7)
		}
	}
	return cfg, nil
}

// resolveIdentityAliasLocked maps an alias name to its configured hex hash.
// Values that are already 32-char hex or are reserved targets pass through.
func (c *ServerConfig) resolveIdentityAliasLocked(value string) string {
	v := strings.TrimSpace(value)
	if resolved, ok := c.IdentityAliases[v]; ok {
		return resolved
	}
	return v
}

// splitListValue splits a ConfigObj-style list value on commas.
func splitListValue(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// EnsureClientConfig writes a default client config when missing.
func EnsureClientConfig(dir string) error {
	if dir == "" {
		dir = DefaultClientConfigDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "client_config")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(defaultClientConfig), 0o600)
}

// EnsureServerConfig writes a default server config when missing.
func EnsureServerConfig(dir string) error {
	if dir == "" {
		dir = DefaultServerConfigDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "config")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(defaultServerConfig), 0o600)
}

func parseINI(path string) (map[string]map[string]string, error) {
	f, err := os.Open(path) // #nosec G304 G703 -- operator config path
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sections := map[string]map[string]string{"": {}}
	current := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSpace(line[1 : len(line)-1])
			if _, ok := sections[current]; !ok {
				sections[current] = map[string]string{}
			}
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if _, ok := sections[current]; !ok {
			sections[current] = map[string]string{}
		}
		sections[current][strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return sections, sc.Err()
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

const defaultClientConfig = `# rngit client config
[client]
ref_batch_size = 25

[aliases]
# my_node = 063d38912bffc850af4a1b8a270a9d85

[logging]
loglevel = 4
`

const defaultServerConfig = `# rngit server config
[rngit]
node_name = Reticulum Git Node
announce_interval = 360
record_stats = no
mirror_interval = 24

# You can block specific identities from any interaction
# with this node.

# blocked_identities = d31aeea49873006f13b3415520666a4e

# To make it easier to handle scrapers, crawlers, slopware
# and other annoyances, you can block unidentified peers by
# adding the null_ident hash to blocked identities.

# blocked_identities = d7db22f63b453c23bb0688dde565b7c1

[repositories]
public = ./repos/public

[access]
public = r:all

[pages]
serve_nomadnet = no

# It is possible to disable Nerd Font icons and instead
# use simpler (but more compatible) unicode icons.

# unicode_icons = yes

# You can configure whether the page server should try
# to convert media files to WebP on the fly, for serving
# to nomadnet clients. Enabled by default, but will
# require an available encoding backend installed on
# your system. Supported backend utilities are "magick",
# "convert", "gm", "ffmpeg" and "avconv". If any one is
# installed, rngit will auto-detect and use it, but you
# can force a specific backend with the environment
# variable RNGIT_MEDIA_BACKEND.

# media_conversion = yes


[logging]
# Valid log levels are 0 through 7:
#   0: Log only critical information
#   1: Log errors and lower log levels
#   2: Log warnings and lower log levels
#   3: Log notices and lower log levels
#   4: Log info and lower (this is the default)
#   5: Verbose logging
#   6: Debug logging
#   7: Extreme logging

loglevel = 4
`

// PrepareGitIdentity loads or creates the rngit client identity file.
func PrepareGitIdentity(path string) (*identity.Identity, error) {
	if path == "" {
		return nil, fmt.Errorf("empty identity path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil { // #nosec G301 G703 -- client identity parent dir
		return nil, err
	}
	if st, err := os.Stat(path); err == nil && !st.IsDir() { // #nosec G703 -- client identity path
		return identity.FromFile(path)
	}
	id, err := identity.New()
	if err != nil {
		return nil, err
	}
	if err := id.ToFile(path); err != nil {
		return nil, err
	}
	return id, nil
}
