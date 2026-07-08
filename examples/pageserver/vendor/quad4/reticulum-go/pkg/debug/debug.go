// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package debug

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"os"
	"sync"
)

var (
	debugLevel  = flag.Int("debug", 3, "debug level (1-7); 1=critical, 2=error, 3=info, 4=verbose, 5=trace, 6=packets, 7=all")
	logger      *slog.Logger
	extraWriter io.Writer
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

// Init builds the underlying slog logger. Safe to call repeatedly. Only

// the first call wires it up. SetDebugLevel rebuilds the handler so the
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
	out := io.Writer(os.Stderr)
	if extraWriter != nil {
		out = io.MultiWriter(os.Stderr, extraWriter)
	}
	logger = slog.New(slog.NewTextHandler(out, opts))
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
