// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package debug

import (
	"bytes"
	"context"
	"flag"
	"log/slog"
	"strings"
	"testing"
)

func resetDebugForTest(t *testing.T, defaultLevel int) {
	t.Helper()
	originalFlag := flag.CommandLine
	t.Cleanup(func() {
		flag.CommandLine = originalFlag
		initialized.Store(false)
		logPtr.Store(nil)
	})
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	debugLevel = flag.Int("debug", defaultLevel, "debug level")
	levelAtomic.Store(int64(*debugLevel))
	initialized.Store(false)
	logPtr.Store(nil)
}

func TestInit(t *testing.T) {
	resetDebugForTest(t, DebugInfo)
	Init()
	if !initialized.Load() {
		t.Error("Init() should set initialized to true")
	}
	if GetLogger() == nil {
		t.Error("GetLogger() should return non-nil logger after Init()")
	}
}

func TestGetLogger(t *testing.T) {
	resetDebugForTest(t, DebugInfo)
	logger := GetLogger()
	if logger == nil {
		t.Error("GetLogger() should return non-nil logger")
	}
	if !initialized.Load() {
		t.Error("GetLogger() should initialize if not already initialized")
	}
}

func TestLog(t *testing.T) {
	resetDebugForTest(t, DebugAll)
	Log(DebugInfo, "test message", "key", "value")
}

func TestSetDebugLevel(t *testing.T) {
	resetDebugForTest(t, DebugInfo)
	SetDebugLevel(5)
	if GetDebugLevel() != 5 {
		t.Errorf("SetDebugLevel(5) did not set level correctly, got %d", GetDebugLevel())
	}
}

func TestGetDebugLevel(t *testing.T) {
	resetDebugForTest(t, DebugVerbose)
	level := GetDebugLevel()
	if level != DebugVerbose {
		t.Errorf("GetDebugLevel() = %d, want %d", level, DebugVerbose)
	}
}

func TestLog_LevelFiltering(t *testing.T) {
	resetDebugForTest(t, DebugInfo)
	Log(DebugTrace, "trace message")
	Log(DebugInfo, "info message")
	Log(DebugError, "error message")
}

func TestConstants(t *testing.T) {
	if DebugCritical != 1 {
		t.Errorf("DebugCritical = %d, want 1", DebugCritical)
	}
	if DebugError != 2 {
		t.Errorf("DebugError = %d, want 2", DebugError)
	}
	if DebugWarning != 3 {
		t.Errorf("DebugWarning = %d, want 3", DebugWarning)
	}
	if DebugInfo != 4 {
		t.Errorf("DebugInfo = %d, want 4", DebugInfo)
	}
	if DebugVerbose != 5 {
		t.Errorf("DebugVerbose = %d, want 5", DebugVerbose)
	}
	if DebugTrace != 6 {
		t.Errorf("DebugTrace = %d, want 6", DebugTrace)
	}
	if DebugPackets != 7 {
		t.Errorf("DebugPackets = %d, want 7", DebugPackets)
	}
	if DebugAll != DebugPackets {
		t.Errorf("DebugAll = %d, want %d", DebugAll, DebugPackets)
	}
}

func TestLevelName(t *testing.T) {
	if got := LevelName(DebugInfo); got != "info" {
		t.Errorf("LevelName(Info)=%q", got)
	}
	if got := LevelName(0); got != "silent" {
		t.Errorf("LevelName(0)=%q", got)
	}
}

func TestClampLevel(t *testing.T) {
	if ClampLevel(-3) != 0 {
		t.Fatalf("negative should clamp to silent")
	}
	if ClampLevel(99) != DebugAll {
		t.Fatalf("oversize should clamp to packets")
	}
	if ClampLevel(DebugInfo) != DebugInfo {
		t.Fatalf("info should pass through")
	}
}

func TestLog_WithArgs(t *testing.T) {
	resetDebugForTest(t, DebugAll)
	Log(DebugInfo, "test message", "key1", "value1", "key2", "value2")
}

func TestInit_MultipleCalls(t *testing.T) {
	resetDebugForTest(t, DebugInfo)
	Init()
	firstLogger := GetLogger()
	Init()
	secondLogger := GetLogger()
	if firstLogger != secondLogger {
		t.Error("Multiple Init() calls should not create new loggers")
	}
}

func TestLog_DisabledLevel(t *testing.T) {
	resetDebugForTest(t, DebugCritical)
	Log(DebugTrace, "this should be filtered")
}

func TestLog_SilentLevel(t *testing.T) {
	resetDebugForTest(t, DebugInfo)
	Init()
	SetDebugLevel(0)
	out := captureLog(t, slog.LevelDebug, func() {
		Log(DebugCritical, "boom")
		Log(DebugInfo, "info")
	})
	if out != "" {
		t.Fatalf("level 0 should silence all output, got %q", out)
	}
}

func TestLog_DoesNotInjectDebugLevelAttr(t *testing.T) {
	resetDebugForTest(t, DebugInfo)
	Init()
	out := captureLog(t, slog.LevelInfo, func() {
		Log(DebugInfo, "plain")
	})
	if strings.Contains(out, "debug_level") {
		t.Fatalf("Log should not append debug_level attr: %q", out)
	}
	if !strings.Contains(out, "plain") {
		t.Fatalf("missing message: %q", out)
	}
}

func TestEnabled_HotPath(t *testing.T) {
	resetDebugForTest(t, DebugInfo)
	if !Enabled(DebugInfo) {
		t.Fatal("info should be enabled at default")
	}
	if Enabled(DebugVerbose) {
		t.Fatal("verbose should be filtered at default info")
	}
}

// captureLog swaps in a buffer-backed slog handler at the given level
// and returns whatever was written during fn.
func captureLog(t *testing.T, level slog.Level, fn func()) string {
	t.Helper()
	mu.Lock()
	prev := logPtr.Load()
	var buf bytes.Buffer
	logPtr.Store(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})))
	initialized.Store(true)
	mu.Unlock()

	defer func() {
		mu.Lock()
		logPtr.Store(prev)
		mu.Unlock()
	}()

	fn()
	return buf.String()
}

func TestSetDebugLevel_SilencesEverythingButCritical(t *testing.T) {
	resetDebugForTest(t, DebugInfo)
	Init()
	SetDebugLevel(DebugCritical)

	out := captureLog(t, slogLevelFor(DebugCritical), func() {
		Log(DebugCritical, "boom")
		Log(DebugError, "err")
		Log(DebugWarning, "warn")
		Log(DebugInfo, "info")
		Log(DebugVerbose, "verbose")
		Log(DebugTrace, "trace")
	})

	if !strings.Contains(out, "boom") {
		t.Fatalf("critical message should pass: %q", out)
	}
	for _, banned := range []string{"err", "warn", "info", "verbose", "trace"} {
		if strings.Contains(out, banned) {
			t.Fatalf("debug level CRITICAL should suppress %q, got: %q", banned, out)
		}
	}
}

func TestSetDebugLevel_RaisesAfterInit(t *testing.T) {
	resetDebugForTest(t, DebugCritical)
	Init()
	SetDebugLevel(DebugTrace)

	out := captureLog(t, slogLevelFor(DebugTrace), func() {
		Log(DebugTrace, "trace-now-on")
	})

	if !strings.Contains(out, "trace-now-on") {
		t.Fatalf("trace should be enabled after raising level: %q", out)
	}
}

func TestSlogLevelFor(t *testing.T) {
	cases := []struct {
		in   int
		want slog.Level
	}{
		{DebugCritical, slog.LevelError},
		{DebugError, slog.LevelError},
		{DebugWarning, slog.LevelWarn},
		{DebugInfo, slog.LevelInfo},
		{DebugVerbose, slog.LevelDebug},
		{DebugTrace, slog.LevelDebug},
		{DebugPackets, slog.LevelDebug},
		{DebugAll, slog.LevelDebug},
	}
	for _, c := range cases {
		if got := slogLevelFor(c.in); got != c.want {
			t.Errorf("slogLevelFor(%d)=%v, want %v", c.in, got, c.want)
		}
	}
	_ = context.Background()
}
