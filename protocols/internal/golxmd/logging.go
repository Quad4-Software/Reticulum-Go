// SPDX-License-Identifier: LicenseRef-Reticulum
package golxmd

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/rnsnode"
)

type LogConfig struct {
	Level     int
	Console   bool
	File      string
	Verbosity int
	Quietness int
}

func effectiveLogLevel(cfg LogConfig) int {
	level := cfg.Level
	if level < lxmf.LogCritical {
		level = lxmf.LogInfo
	}
	if level > lxmf.LogExtreme {
		level = lxmf.LogExtreme
	}
	level += cfg.Verbosity - cfg.Quietness
	if level < lxmf.LogCritical {
		level = lxmf.LogCritical
	}
	if level > lxmf.LogExtreme {
		level = lxmf.LogExtreme
	}
	return level
}

func ConfigureLogging(cfg LogConfig) (*slog.Logger, io.Closer, error) {
	level := effectiveLogLevel(cfg)
	lxmf.SetLogLevel(level)
	lxmf.MirrorRNSDebug(false)
	debug.Init()
	debug.SetDebugLevel(rnsDebugLevel(level))

	slogLevel := slogLevelForLXMF(level)
	var writers []io.Writer
	var closer io.Closer
	if cfg.File != "" {
		p := expandPath(cfg.File)
		if err := ensurePrivateDir(filepath.Dir(p)); err != nil {
			return nil, nil, err
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 -- operator log path
		if err != nil {
			return nil, nil, err
		}
		writers = append(writers, f)
		closer = f

		if err := debug.ConfigureDestination(&common.ReticulumConfig{
			LogDestination: "file",
			LogFile:        p,
		}); err != nil {
			_ = f.Close()
			return nil, nil, err
		}
	}
	if cfg.Console {
		writers = append(writers, os.Stderr)
	}
	if len(writers) == 0 {
		writers = append(writers, os.Stderr)
	}
	w := io.MultiWriter(writers...)
	lxmf.SetLogOutput(w)
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slogLevel})
	return slog.New(h), closer, nil
}

func rnsDebugLevel(level int) int {
	switch {
	case level <= lxmf.LogError:
		return 2
	case level <= lxmf.LogWarning:
		return 2
	case level <= lxmf.LogInfo:
		return 3
	case level <= lxmf.LogVerbose:
		return 4
	case level <= lxmf.LogDebug:
		return 5
	default:
		return 6
	}
}

func slogLevelForLXMF(level int) slog.Level {
	switch {
	case level <= lxmf.LogCritical:
		return slog.LevelError + 4
	case level <= lxmf.LogError:
		return slog.LevelError
	case level <= lxmf.LogWarning:
		return slog.LevelWarn
	case level <= lxmf.LogInfo:
		return slog.LevelInfo
	case level <= lxmf.LogVerbose:
		return slog.LevelDebug
	default:
		return slog.LevelDebug - 4
	}
}

func FirstRun(home, configPath, identityPath, storageDir, messagesDir, rnsDir string) (bool, error) {
	created := false
	if err := ensurePrivateDir(home); err != nil {
		return false, err
	}
	if err := ensurePrivateDir(storageDir); err != nil {
		return false, err
	}
	if err := ensurePrivateDir(messagesDir); err != nil {
		return false, err
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := lxmf.WriteDefaultConfigFile(configPath); err != nil {
			return false, err
		}
		created = true
	}
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		if err := ensurePrivateDir(filepath.Dir(identityPath)); err != nil {
			return false, err
		}
		id, err := identity.NewIdentity()
		if err != nil {
			return false, err
		}
		if err := id.ToFile(identityPath); err != nil {
			return false, err
		}
		_ = os.Chmod(identityPath, 0o600)
		created = true
	}
	if _, err := rnsnode.EnsureDefaultConfig(ResolveRNSConfigDir(rnsDir)); err != nil {
		return false, err
	}
	return created, nil
}
