// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"os"
	"path/filepath"
	"strings"
)

func DefaultHome() string {
	if v := os.Getenv("GOLXMD_HOME"); v != "" {
		return expandPath(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".golxmd"
	}
	return filepath.Join(home, ".golxmd")
}

func DefaultConfigPath() string {
	return filepath.Join(DefaultHome(), "config")
}

func DefaultIdentityPath() string {
	return filepath.Join(DefaultHome(), "identity")
}

func DefaultStorageDir() string {
	return filepath.Join(DefaultHome(), "storage")
}

func DefaultMessagesDir() string {
	return filepath.Join(DefaultStorageDir(), "messages")
}

func DefaultIgnoredPath() string {
	return filepath.Join(DefaultHome(), "ignored")
}

func DefaultAllowedPath() string {
	return filepath.Join(DefaultHome(), "allowed")
}

func DefaultReadyPath() string {
	return filepath.Join(DefaultHome(), "ready.json")
}

func DefaultLogPath() string {
	return filepath.Join(DefaultHome(), "logfile")
}

// DefaultRNSConfigDir is the Reticulum-Go config directory when --rnsconfig is omitted.
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

func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	p = os.ExpandEnv(p)
	if strings.HasPrefix(p, "~"+string(os.PathSeparator)) || p == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
