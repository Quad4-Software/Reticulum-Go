// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"os"
	"path/filepath"
	"strings"
)

func DefaultHome() string {
	if v := os.Getenv("GORRCD_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".gorrcd"
	}
	return filepath.Join(home, ".gorrcd")
}

func DefaultConfigPath() string {
	return filepath.Join(DefaultHome(), "gorrcd.toml")
}

func DefaultIdentityPath() string {
	return filepath.Join(DefaultHome(), "hub_identity")
}

func DefaultRoomsPath() string {
	return filepath.Join(DefaultHome(), "rooms.toml")
}

// DefaultRNSConfigDir is the Reticulum-Go config directory when configdir is unset.
func DefaultRNSConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".reticulum-go")
}

// ResolveRNSConfigDir returns override when set, otherwise DefaultRNSConfigDir.
func ResolveRNSConfigDir(override string) string {
	override = strings.TrimSpace(override)
	if override != "" {
		return expandPath(override)
	}
	return DefaultRNSConfigDir()
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(path, 0o700) // #nosec G302 -- directories need the execute bit
	return nil
}
