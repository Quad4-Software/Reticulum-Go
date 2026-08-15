// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package harness

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadLineTimeoutNoConcurrentPanic(t *testing.T) {
	r, w := io.Pipe()
	br := bufio.NewReader(r)
	ctx := context.Background()

	if _, err := ReadLineTimeout(ctx, br, 20*time.Millisecond); err != context.DeadlineExceeded {
		t.Fatalf("first wait err=%v want deadline exceeded", err)
	}

	go func() {
		_, _ = io.WriteString(w, "hello\n")
		_ = w.Close()
	}()

	line, err := ReadLineTimeout(ctx, br, time.Second)
	if err != nil {
		t.Fatalf("second wait: %v", err)
	}
	if strings.TrimSpace(line) != "hello" {
		t.Fatalf("got %q", line)
	}
}

func TestEventLogIngestAndLast(t *testing.T) {
	dir := t.TempDir()
	ev, err := NewEventLog(dir)
	if err != nil {
		t.Fatalf("NewEventLog: %v", err)
	}
	ev.Emit("ready", "", "go side", nil)
	ok := ev.IngestPythonLine(`INTEROP_EVENT {"ts":"2026-01-01T00:00:00Z","src":"py","event":"path_ok","detail":"abc"}`)
	if !ok {
		t.Fatal("expected ingest ok")
	}
	if ev.IngestPythonLine("not an event") {
		t.Fatal("expected non-event rejected")
	}
	last := ev.Last(2)
	if len(last) != 2 {
		t.Fatalf("last len=%d", len(last))
	}
	if last[0].Event != "ready" || last[1].Event != "path_ok" {
		t.Fatalf("unexpected events %#v", last)
	}
	raw, err := os.ReadFile(ev.Path())
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("jsonl lines=%d", len(lines))
	}
	var parsed Event
	if err := json.Unmarshal([]byte(lines[1]), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Src != "py" || parsed.Detail != "abc" {
		t.Fatalf("parsed %#v", parsed)
	}
}

func TestWriteArtifacts(t *testing.T) {
	dir := t.TempDir()
	if err := WriteStderrCapture(dir, "boom\n"); err != nil {
		t.Fatalf("stderr: %v", err)
	}
	if err := WriteEnvSnapshot(dir, map[string]string{"FOO": "bar"}); err != nil {
		t.Fatalf("env: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stderr.txt")); err != nil {
		t.Fatalf("stderr missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "env.json")); err != nil {
		t.Fatalf("env missing: %v", err)
	}
}

func TestKindString(t *testing.T) {
	if Kind("").String() != "harness" {
		t.Fatalf("empty kind")
	}
	if KindPath.String() != "path" {
		t.Fatalf("path kind")
	}
}
