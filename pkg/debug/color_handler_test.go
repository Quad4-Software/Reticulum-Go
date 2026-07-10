// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package debug

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestColorizeLevelBytes(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "")

	in := []byte(`time=2026-07-09T14:48:21.731-05:00 level=ERROR msg="hi"`)
	out := colorizeLevelBytes(in, slog.LevelError, nil)
	if strings.Contains(string(out), `\x1b`) {
		t.Fatalf("escaped ANSI in output: %q", out)
	}
	if !bytes.Contains(out, []byte("\033[31mERROR\033[0m")) {
		t.Fatalf("expected raw red ERROR, got %q", out)
	}
	if bytes.Contains(out, []byte(`level="`)) {
		t.Fatalf("level should not be quoted: %q", out)
	}
}

func TestColorHandlerHandle(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "1")

	var buf bytes.Buffer
	h := newColorHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	r := slog.NewRecord(time.Now(), slog.LevelWarn, "Configuring interface", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if strings.Contains(s, `\x1b`) || strings.Contains(s, `level="`) {
		t.Fatalf("bad colored line: %q", s)
	}
	if !strings.Contains(s, "\033[33mWARN\033[0m") {
		t.Fatalf("expected yellow WARN, got %q", s)
	}
}

func TestUseColorLogsGates(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "1")

	mu.Lock()
	prevJSON := jsonFormat
	prevOmit := omitStderr
	prevExtra := extraWriter
	jsonFormat = false
	omitStderr = false
	extraWriter = nil
	ok := useColorLogs()
	jsonFormat = true
	jsonBlocks := useColorLogs()
	jsonFormat = false
	extraWriter = &bytes.Buffer{}
	extraBlocks := useColorLogs()
	jsonFormat = prevJSON
	omitStderr = prevOmit
	extraWriter = prevExtra
	mu.Unlock()

	if !ok {
		t.Fatal("expected color when FORCE_COLOR and stderr-only text")
	}
	if jsonBlocks {
		t.Fatal("JSON must not color")
	}
	if extraBlocks {
		t.Fatal("extra writer must not color")
	}
}
