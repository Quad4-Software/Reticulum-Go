// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	rnsdebug "git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
)

// Log levels match RNS debug (1-7) so one -debug flag can control both stacks.
const (
	LogCritical = 1
	LogError    = 2
	LogWarning  = 3
	LogNotice   = 4
	LogInfo     = 4
	LogVerbose  = 5
	LogDebug    = 6
	LogExtreme  = 7
)

// LogSubsystem is the slog subsystem attribute for LXMF records.
const LogSubsystem = "lxmf"

var (
	logLevel    atomic.Int32
	logMu       sync.RWMutex
	logger      *slog.Logger
	mirrorRNS   atomic.Bool
	logLevelSet atomic.Bool
)

func init() {
	logLevel.Store(int32(LogInfo))
	mirrorRNS.Store(true)
	rebuildLogger(os.Stderr)
}

// SetLogLevel sets the minimum log level. Higher numeric levels are discarded.
func SetLogLevel(level int) {
	if level < LogCritical {
		level = LogCritical
	}
	if level > LogExtreme {
		level = LogExtreme
	}
	logLevel.Store(int32(level))
	logLevelSet.Store(true)
	logMu.Lock()
	rebuildLoggerLocked(currentWriter())
	logMu.Unlock()
}

// LogLevel returns the effective threshold, optionally mirroring RNS debug until SetLogLevel runs.
func LogLevel() int {
	if mirrorRNS.Load() && !logLevelSet.Load() {
		return rnsdebug.GetDebugLevel()
	}
	return int(logLevel.Load())
}

// MirrorRNSDebug toggles inheriting RNS debug level (default true). SetLogLevel overrides when used.
func MirrorRNSDebug(enabled bool) {
	mirrorRNS.Store(enabled)
}

// SetLogOutput sets the log writer. Nil means os.Stderr.
func SetLogOutput(w io.Writer) {
	if w == nil {
		w = os.Stderr
	}
	logMu.Lock()
	rebuildLoggerLocked(w)
	logMu.Unlock()
}

// SetLogger replaces the default logger. Ignores MirrorRNSDebug and SetLogOutput.
func SetLogger(l *slog.Logger) {
	if l == nil {
		l = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	logMu.Lock()
	logger = l.With(slog.String("subsystem", LogSubsystem))
	logMu.Unlock()
}

// Logger returns the active slog.Logger with the LXMF subsystem field.
func Logger() *slog.Logger {
	logMu.RLock()
	defer logMu.RUnlock()
	return logger
}

// Log writes at level using slog-style args. Dropped if level is above the threshold.
func Log(level int, msg string, args ...any) {
	if level > LogLevel() {
		return
	}
	logMu.RLock()
	l := logger
	logMu.RUnlock()
	l.Log(context.Background(), slogLevelFor(level), msg, args...)
}

// Critical emits a record at LogCritical.
func Critical(msg string, args ...any) { Log(LogCritical, msg, args...) }

// Error emits a record at LogError.
func Error(msg string, args ...any) { Log(LogError, msg, args...) }

// Warning emits a record at LogWarning.
func Warning(msg string, args ...any) { Log(LogWarning, msg, args...) }

// Notice emits a record at LogNotice.
func Notice(msg string, args ...any) { Log(LogNotice, msg, args...) }

// Info emits a record at LogInfo.
func Info(msg string, args ...any) { Log(LogInfo, msg, args...) }

// Verbose emits a record at LogVerbose.
func Verbose(msg string, args ...any) { Log(LogVerbose, msg, args...) }

// Debug emits a record at LogDebug.
func Debug(msg string, args ...any) { Log(LogDebug, msg, args...) }

// Extreme emits a record at LogExtreme.
func Extreme(msg string, args ...any) { Log(LogExtreme, msg, args...) }

func slogLevelFor(level int) slog.Level {
	switch {
	case level <= LogCritical:
		return slog.LevelError + 4
	case level <= LogError:
		return slog.LevelError
	case level <= LogWarning:
		return slog.LevelWarn
	case level <= LogInfo:
		return slog.LevelInfo
	case level <= LogVerbose:
		return slog.LevelDebug
	default:
		return slog.LevelDebug - 4
	}
}

type lxmfHandler struct {
	inner slog.Handler
}

func (h *lxmfHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *lxmfHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.inner.Handle(ctx, r)
}

func (h *lxmfHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &lxmfHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *lxmfHandler) WithGroup(name string) slog.Handler {
	return &lxmfHandler{inner: h.inner.WithGroup(name)}
}

var currentWriterRef atomic.Value

func rebuildLogger(w io.Writer) {
	logMu.Lock()
	rebuildLoggerLocked(w)
	logMu.Unlock()
}

func rebuildLoggerLocked(w io.Writer) {
	currentWriterRef.Store(writerHolder{w: w})
	opts := &slog.HandlerOptions{Level: slog.LevelDebug - 4}
	base := slog.NewTextHandler(w, opts)
	logger = slog.New(&lxmfHandler{inner: base}).With(slog.String("subsystem", LogSubsystem))
}

type writerHolder struct{ w io.Writer }

func currentWriter() io.Writer {
	v := currentWriterRef.Load()
	if v == nil {
		return os.Stderr
	}
	return v.(writerHolder).w
}
