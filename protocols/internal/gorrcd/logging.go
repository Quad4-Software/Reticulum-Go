// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/rnsnode"
)

func ConfigureLogging(cfg Config) (*slog.Logger, io.Closer, error) {
	level := slog.LevelInfo
	switch strings.ToUpper(cfg.LogLevel) {
	case "DEBUG":
		level = slog.LevelDebug
		debug.SetDebugLevel(5)
	case "WARNING", "WARN":
		level = slog.LevelWarn
		debug.SetDebugLevel(2)
	case "ERROR":
		level = slog.LevelError
		debug.SetDebugLevel(2)
	default:
		debug.SetDebugLevel(3)
	}
	switch strings.ToUpper(cfg.LogRNSLevel) {
	case "DEBUG":
		debug.SetDebugLevel(5)
	case "INFO":
		debug.SetDebugLevel(3)
	case "ERROR":
		debug.SetDebugLevel(2)
	}

	var writers []io.Writer
	if cfg.LogConsole {
		writers = append(writers, os.Stderr)
	}
	var closer io.Closer
	if cfg.LogFile != "" {
		p := expandPath(cfg.LogFile)
		if err := ensurePrivateDir(filepath.Dir(p)); err != nil {
			return nil, nil, err
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 -- operator log path
		if err != nil {
			return nil, nil, err
		}
		writers = append(writers, f)
		closer = f
	}
	if len(writers) == 0 {
		writers = append(writers, os.Stderr)
	}
	w := io.MultiWriter(writers...)
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(h), closer, nil
}

func FirstRun(cfgPath, identPath, roomsPath, rnsDir string) (bool, error) {
	created := false
	if err := ensurePrivateDir(filepath.Dir(cfgPath)); err != nil {
		return false, err
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		c := DefaultConfig()
		c.ConfigPath = cfgPath
		c.IdentityPath = identPath
		c.RoomRegistryPath = roomsPath
		if err := WriteConfigFile(c, cfgPath); err != nil {
			return false, err
		}
		created = true
	}
	if _, err := os.Stat(identPath); os.IsNotExist(err) {
		if err := ensurePrivateDir(filepath.Dir(identPath)); err != nil {
			return false, err
		}
		id, err := newIdentityFile(identPath)
		if err != nil {
			return false, err
		}
		_ = id
		created = true
	}
	if roomsPath != "" {
		if _, err := os.Stat(roomsPath); os.IsNotExist(err) {
			if err := ensurePrivateDir(filepath.Dir(roomsPath)); err != nil {
				return false, err
			}
			text := "# gorrcd room registry\n\n[rooms]\n"
			if err := atomicWrite(roomsPath, []byte(text), 0o600); err != nil {
				return false, err
			}
			created = true
		}
	}
	if _, err := rnsnode.EnsureDefaultConfig(ResolveRNSConfigDir(rnsDir)); err != nil {
		return false, err
	}
	return created, nil
}

func newIdentityFile(path string) (string, error) {
	id, err := identity.NewIdentity()
	if err != nil {
		return "", err
	}
	if err := id.ToFile(path); err != nil {
		return "", err
	}
	_ = os.Chmod(path, 0o600)
	return fmt.Sprintf("%x", id.Hash()), nil
}
