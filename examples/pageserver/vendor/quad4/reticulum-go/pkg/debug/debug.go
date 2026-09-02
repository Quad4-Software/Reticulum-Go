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
	"sync/atomic"

	"quad4/reticulum-go/pkg/common"
)

var (
	debugLevel  = flag.Int("debug", DebugInfo, "debug level (0-7) 0=silent 1=critical 2=error 3=warning 4=info 5=verbose 6=trace 7=packets")
	logPtr      atomic.Pointer[slog.Logger]
	extraWriter io.Writer
	jsonFormat  bool
	omitStderr  bool
	logFile     *os.File
	initialized atomic.Bool
	mu          sync.Mutex

	// levelAtomic mirrors *debugLevel so hot-path callers can check
	// debug.Enabled(level) without a mutex and without evaluating
	// expensive log arguments when the message would be filtered.
	levelAtomic atomic.Int64
)

func init() {
	levelAtomic.Store(int64(*debugLevel))
}

// SetExtraWriter mirrors Reticulum log output to w in addition to stderr.
func SetExtraWriter(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	extraWriter = w
	if initialized.Load() {
		rebuildLocked()
	}
}

// SetJSONFormat switches between text and JSON slog handlers.
func SetJSONFormat(enabled bool) {
	mu.Lock()
	defer mu.Unlock()
	jsonFormat = enabled
	if initialized.Load() {
		rebuildLocked()
	}
}

// Init builds the underlying slog logger. Safe to call repeatedly.
// Only the first call wires it up. SetDebugLevel rebuilds the handler so the
// active level can change at runtime.
func Init() {
	mu.Lock()
	defer mu.Unlock()
	if initialized.Load() {
		return
	}
	levelAtomic.Store(int64(*debugLevel))
	rebuildLocked()
	initialized.Store(true)
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
	var l *slog.Logger
	if jsonFormat {
		l = slog.New(slog.NewJSONHandler(out, opts))
	} else if useColorLogs() {
		l = slog.New(newColorHandler(out, opts))
	} else {
		l = slog.New(slog.NewTextHandler(out, opts))
	}
	logPtr.Store(l)
	slog.SetDefault(l)
}

// slogLevelFor maps an RNS debug level to the closest slog level.
func slogLevelFor(level int) slog.Level {
	switch {
	case level >= DebugVerbose:
		return slog.LevelDebug
	case level >= DebugInfo:
		return slog.LevelInfo
	case level >= DebugWarning:
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}

// GetLogger returns the underlying slog logger. Prefer Log so callers
// route through the central level filter.
func GetLogger() *slog.Logger {
	if l := logPtr.Load(); l != nil {
		return l
	}
	Init()
	return logPtr.Load()
}

// Log emits msg at the given RNS debug level, suppressing it when the
// level is above the current threshold.
func Log(level int, msg string, args ...any) {
	if int(levelAtomic.Load()) < level {
		return
	}
	l := logPtr.Load()
	if l == nil {
		Init()
		l = logPtr.Load()
		if l == nil {
			return
		}
	}
	l.Log(context.Background(), slogLevelFor(level), msg, args...)
}

// SetDebugLevel updates the active level and rebuilds the slog handler
// so the change takes effect immediately. Values below 1 silence output.
func SetDebugLevel(level int) {
	level = ClampLevel(level)
	mu.Lock()
	defer mu.Unlock()
	*debugLevel = level
	levelAtomic.Store(int64(level))
	if initialized.Load() {
		rebuildLocked()
	}
}

// GetDebugLevel returns the current debug level.
func GetDebugLevel() int {
	return int(levelAtomic.Load())
}

// Enabled reports whether messages at level would be emitted. Hot paths
// should call this before constructing expensive log arguments (e.g.
// fmt.Sprintf) to avoid per-call allocations on a busy network running
// below DebugPackets.
func Enabled(level int) bool {
	return int(levelAtomic.Load()) >= level
}

// ConfigureDestination applies [logging] destination and logfile from cfg.
// destination values: stderr (default), file, both, syslog, journald.
// Combinations such as syslog+stderr and journald+file are also accepted.
func ConfigureDestination(cfg *common.ReticulumConfig) error {
	if cfg == nil {
		return nil
	}
	dest := strings.ToLower(strings.TrimSpace(cfg.LogDestination))
	if dest == "" {
		dest = "stderr"
	}
	parts := splitLogDestinations(dest)

	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
	extraWriter = nil
	omitStderr = false

	var writers []io.Writer
	wantStderr := false
	wantFile := false
	wantSyslog := false
	wantJournald := false
	for _, p := range parts {
		switch p {
		case "stderr", "stdout":
			wantStderr = true
		case "file":
			wantFile = true
		case "both":
			wantStderr = true
			wantFile = true
		case "syslog":
			wantSyslog = true
		case "journald", "journal":
			wantJournald = true
		}
	}
	if !wantStderr && !wantFile && !wantSyslog && !wantJournald {
		wantStderr = true
	}

	if wantFile {
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
		writers = append(writers, f)
	}

	tag := "reticulum-go"
	if wantSyslog {
		w, err := openSyslogWriter(tag)
		if err != nil {
			return err
		}
		writers = append(writers, w)
	}
	if wantJournald {
		w, err := openJournaldWriter(tag)
		if err != nil {
			return err
		}
		writers = append(writers, w)
	}

	switch {
	case len(writers) == 0:
		omitStderr = false
		extraWriter = nil
	case !wantStderr:
		omitStderr = true
		if len(writers) == 1 {
			extraWriter = writers[0]
		} else {
			extraWriter = io.MultiWriter(writers...)
		}
	default:
		omitStderr = false
		if len(writers) == 1 {
			extraWriter = writers[0]
		} else if len(writers) > 1 {
			extraWriter = io.MultiWriter(writers...)
		}
	}

	if strings.EqualFold(cfg.LogFormat, "json") {
		jsonFormat = true
	}

	if initialized.Load() {
		rebuildLocked()
	}
	return nil
}

func splitLogDestinations(dest string) []string {
	dest = strings.ReplaceAll(dest, "+", ",")
	dest = strings.ReplaceAll(dest, "|", ",")
	raw := strings.Split(dest, ",")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
