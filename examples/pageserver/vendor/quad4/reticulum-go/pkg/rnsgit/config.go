// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"bufio"
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
	RNSConfigDir      string
	IdentityPath      string
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
		IdentityPath:      filepath.Join(dir, "server_identity"),
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
	return cfg, nil
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

[repositories]
public = ./repos/public

[access]
public = r:all

[pages]
serve_nomadnet = no
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
