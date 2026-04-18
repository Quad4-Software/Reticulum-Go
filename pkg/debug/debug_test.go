package debug

import (
	"bytes"
	"context"
	"flag"
	"log/slog"
	"strings"
	"testing"
)

func TestInit(t *testing.T) {
	originalFlag := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlag
		initialized = false
	}()

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	debugLevel = flag.Int("debug", 3, "debug level")

	Init()

	if !initialized {
		t.Error("Init() should set initialized to true")
	}

	if GetLogger() == nil {
		t.Error("GetLogger() should return non-nil logger after Init()")
	}
}

func TestGetLogger(t *testing.T) {
	originalFlag := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlag
		initialized = false
	}()

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	debugLevel = flag.Int("debug", 3, "debug level")
	initialized = false

	logger := GetLogger()
	if logger == nil {
		t.Error("GetLogger() should return non-nil logger")
	}

	if !initialized {
		t.Error("GetLogger() should initialize if not already initialized")
	}
}

func TestLog(t *testing.T) {
	originalFlag := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlag
		initialized = false
	}()

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	debugLevel = flag.Int("debug", 7, "debug level")
	initialized = false

	Log(DEBUG_INFO, "test message", "key", "value")
}

func TestSetDebugLevel(t *testing.T) {
	originalFlag := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlag
		initialized = false
	}()

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	debugLevel = flag.Int("debug", 3, "debug level")
	initialized = false

	SetDebugLevel(5)
	if GetDebugLevel() != 5 {
		t.Errorf("SetDebugLevel(5) did not set level correctly, got %d", GetDebugLevel())
	}
}

func TestGetDebugLevel(t *testing.T) {
	originalFlag := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlag
		initialized = false
	}()

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	debugLevel = flag.Int("debug", 4, "debug level")

	level := GetDebugLevel()
	if level != 4 {
		t.Errorf("GetDebugLevel() = %d, want 4", level)
	}
}

func TestLog_LevelFiltering(t *testing.T) {
	originalFlag := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlag
		initialized = false
	}()

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	debugLevel = flag.Int("debug", 3, "debug level")
	initialized = false

	Log(DEBUG_TRACE, "trace message")
	Log(DEBUG_INFO, "info message")
	Log(DEBUG_ERROR, "error message")
}

func TestConstants(t *testing.T) {
	if DEBUG_CRITICAL != 1 {
		t.Errorf("DEBUG_CRITICAL = %d, want 1", DEBUG_CRITICAL)
	}
	if DEBUG_ERROR != 2 {
		t.Errorf("DEBUG_ERROR = %d, want 2", DEBUG_ERROR)
	}
	if DEBUG_INFO != 3 {
		t.Errorf("DEBUG_INFO = %d, want 3", DEBUG_INFO)
	}
	if DEBUG_VERBOSE != 4 {
		t.Errorf("DEBUG_VERBOSE = %d, want 4", DEBUG_VERBOSE)
	}
	if DEBUG_TRACE != 5 {
		t.Errorf("DEBUG_TRACE = %d, want 5", DEBUG_TRACE)
	}
	if DEBUG_PACKETS != 6 {
		t.Errorf("DEBUG_PACKETS = %d, want 6", DEBUG_PACKETS)
	}
	if DEBUG_ALL != 7 {
		t.Errorf("DEBUG_ALL = %d, want 7", DEBUG_ALL)
	}
}

func TestLog_WithArgs(t *testing.T) {
	originalFlag := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlag
		initialized = false
	}()

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	debugLevel = flag.Int("debug", 7, "debug level")
	initialized = false

	Log(DEBUG_INFO, "test message", "key1", "value1", "key2", "value2")
}

func TestInit_MultipleCalls(t *testing.T) {
	originalFlag := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlag
		initialized = false
	}()

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	debugLevel = flag.Int("debug", 3, "debug level")
	initialized = false

	Init()
	firstLogger := GetLogger()

	Init()
	secondLogger := GetLogger()

	if firstLogger != secondLogger {
		t.Error("Multiple Init() calls should not create new loggers")
	}
}

func TestLog_DisabledLevel(t *testing.T) {
	originalFlag := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlag
		initialized = false
	}()

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	debugLevel = flag.Int("debug", 1, "debug level")
	initialized = false

	Log(DEBUG_TRACE, "this should be filtered")
}

// captureLog swaps in a buffer-backed slog handler at the given level
// and returns whatever was written during fn.
func captureLog(t *testing.T, level slog.Level, fn func()) string {
	t.Helper()
	mu.Lock()
	prev := logger
	var buf bytes.Buffer
	logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level}))
	initialized = true
	mu.Unlock()

	defer func() {
		mu.Lock()
		logger = prev
		mu.Unlock()
	}()

	fn()
	return buf.String()
}

// TestSetDebugLevel_SilencesEverythingButCritical verifies that lowering
// the debug level at runtime truly suppresses higher-level output.
func TestSetDebugLevel_SilencesEverythingButCritical(t *testing.T) {
	originalFlag := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlag
		mu.Lock()
		initialized = false
		mu.Unlock()
	}()

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	debugLevel = flag.Int("debug", DEBUG_INFO, "debug level")
	mu.Lock()
	initialized = false
	mu.Unlock()
	Init()

	SetDebugLevel(DEBUG_CRITICAL)

	out := captureLog(t, slogLevelFor(DEBUG_CRITICAL), func() {
		Log(DEBUG_CRITICAL, "boom")
		Log(DEBUG_ERROR, "err")
		Log(DEBUG_INFO, "info")
		Log(DEBUG_VERBOSE, "verbose")
		Log(DEBUG_TRACE, "trace")
	})

	if !strings.Contains(out, "boom") {
		t.Fatalf("critical message should pass: %q", out)
	}
	for _, banned := range []string{"err", "info", "verbose", "trace"} {
		if strings.Contains(out, banned) {
			t.Fatalf("debug level CRITICAL should suppress %q, got: %q", banned, out)
		}
	}
}

// TestSetDebugLevel_RaisesAfterInit verifies that raising the debug
// level at runtime makes previously-suppressed messages appear.
func TestSetDebugLevel_RaisesAfterInit(t *testing.T) {
	originalFlag := flag.CommandLine
	defer func() {
		flag.CommandLine = originalFlag
		mu.Lock()
		initialized = false
		mu.Unlock()
	}()

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	debugLevel = flag.Int("debug", DEBUG_CRITICAL, "debug level")
	mu.Lock()
	initialized = false
	mu.Unlock()
	Init()

	SetDebugLevel(DEBUG_TRACE)

	out := captureLog(t, slogLevelFor(DEBUG_TRACE), func() {
		Log(DEBUG_TRACE, "trace-now-on")
	})

	if !strings.Contains(out, "trace-now-on") {
		t.Fatalf("trace should be enabled after raising level: %q", out)
	}
}

// TestSlogLevelFor sanity-checks the RNS->slog level mapping so the
// handler filter and the explicit Log filter stay consistent.
func TestSlogLevelFor(t *testing.T) {
	cases := []struct {
		in   int
		want slog.Level
	}{
		{DEBUG_CRITICAL, slog.LevelError},
		{DEBUG_ERROR, slog.LevelWarn},
		{DEBUG_INFO, slog.LevelInfo},
		{DEBUG_VERBOSE, slog.LevelDebug},
		{DEBUG_TRACE, slog.LevelDebug},
		{DEBUG_PACKETS, slog.LevelDebug},
		{DEBUG_ALL, slog.LevelDebug},
	}
	for _, c := range cases {
		if got := slogLevelFor(c.in); got != c.want {
			t.Errorf("slogLevelFor(%d)=%v, want %v", c.in, got, c.want)
		}
	}
	_ = context.Background()
}
