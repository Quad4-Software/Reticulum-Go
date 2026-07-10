// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package debug

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"quad4/reticulum-go/pkg/common"
)

var (
	debugLevel  = flag.Int("debug", 3, "debug level (1-7); 1=critical, 2=error, 3=info, 4=verbose, 5=trace, 6=packets, 7=all")
	logger      *slog.Logger
	extraWriter io.Writer
	jsonFormat  bool
	omitStderr  bool
	logFile     *os.File
	initialized bool
	mu          sync.RWMutex
)

// SetExtraWriter mirrors Reticulum log output to w in addition to stderr.
func SetExtraWriter(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	extraWriter = w
	if initialized {
		rebuildLocked()
	}
}

// SetJSONFormat switches between text and JSON slog handlers.
func SetJSONFormat(enabled bool) {
	mu.Lock()
	defer mu.Unlock()
	jsonFormat = enabled
	if initialized {
		rebuildLocked()
	}
}

// Init builds the underlying slog logger. Safe to call repeatedly.
// Only the first call wires it up. SetDebugLevel rebuilds the handler so the
// active level can change at runtime.
func Init() {
	mu.Lock()
	defer mu.Unlock()
	if initialized {
		return
	}
	rebuildLocked()
	initialized = true
}

// rebuildLocked rebuilds the slog logger so the handler honours the
// current *debugLevel. Caller must hold mu.
func rebuildLocked() {
	opts := &slog.HandlerOptions{Level: slogLevelFor(*debugLevel)}
	var out io.Writer
	switch {
	case omitStderr && extraWriter != nil:
		out = extraWriter
	case extraWriter != nil:
		out = io.MultiWriter(os.Stderr, extraWriter)
	default:
		out = os.Stderr
	}
	if jsonFormat {
		logger = slog.New(slog.NewJSONHandler(out, opts))
	} else if useColorLogs() {
		logger = slog.New(newColorHandler(out, opts))
	} else {
		logger = slog.New(slog.NewTextHandler(out, opts))
	}
	slog.SetDefault(logger)
}

// slogLevelFor maps an RNS debug level (1-7) to the closest slog level.
func slogLevelFor(level int) slog.Level {
	switch {
	case level >= DebugVerbose:
		return slog.LevelDebug
	case level >= DebugInfo:
		return slog.LevelInfo
	case level >= DebugError:
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}

// GetLogger returns the underlying slog logger. Prefer Log so callers
// route through the central level filter.
func GetLogger() *slog.Logger {
	mu.RLock()
	if initialized {
		l := logger
		mu.RUnlock()
		return l
	}
	mu.RUnlock()
	Init()
	mu.RLock()
	defer mu.RUnlock()
	return logger
}

// Log emits msg at the given RNS debug level, suppressing it when the
// level is above the current threshold.
func Log(level int, msg string, args ...any) {
	mu.RLock()
	ready := initialized
	mu.RUnlock()
	if !ready {
		Init()
	}

	mu.RLock()
	if *debugLevel < level {
		mu.RUnlock()
		return
	}
	l := logger
	mu.RUnlock()

	slogLevel := slogLevelFor(level)
	if !l.Enabled(context.TODO(), slogLevel) {
		return
	}

	allArgs := make([]any, len(args)+2)
	copy(allArgs, args)
	allArgs[len(args)] = "debug_level"
	allArgs[len(args)+1] = level
	l.Log(context.TODO(), slogLevel, msg, allArgs...)
}

// SetDebugLevel updates the active level and rebuilds the slog handler
// so the change takes effect immediately.
func SetDebugLevel(level int) {
	mu.Lock()
	defer mu.Unlock()
	*debugLevel = level
	if initialized {
		rebuildLocked()
	}
}

// GetDebugLevel returns the current debug level.
func GetDebugLevel() int {
	return *debugLevel
}

// ConfigureDestination applies [logging] destination and logfile from cfg.
// destination values: stderr (default), file, both.
func ConfigureDestination(cfg *common.ReticulumConfig) error {
	if cfg == nil {
		return nil
	}
	dest := strings.ToLower(strings.TrimSpace(cfg.LogDestination))
	if dest == "" {
		dest = "stderr"
	}

	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
	extraWriter = nil
	omitStderr = false

	needFile := dest == "file" || dest == "both"
	if needFile {
		path := strings.TrimSpace(cfg.LogFile)
		if path == "" {
			base := ""
			if cfg.ConfigPath != "" {
				base = filepath.Dir(cfg.ConfigPath)
			} else {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				base = filepath.Join(home, ".reticulum-go")
			}
			path = filepath.Join(base, "logfile", "reticulum.log")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil { // #nosec G301
			return fmt.Errorf("logfile dir: %w", err)
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304
		if err != nil {
			return fmt.Errorf("open logfile: %w", err)
		}
		logFile = f
		extraWriter = f
		if dest == "file" {
			omitStderr = true
		}
	}

	if strings.EqualFold(cfg.LogFormat, "json") {
		jsonFormat = true
	}

	if initialized {
		rebuildLocked()
	}
	return nil
}
