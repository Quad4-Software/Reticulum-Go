// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sandbox

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"
)

func TestWarnSoftUnavailableRateLimit(t *testing.T) {
	lastWarn = make(map[string]time.Time)
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	warnSoftUnavailable("landlock", "test reason")
	warnSoftUnavailable("landlock", "test reason")
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("WARNING: sandbox soft-unavailable mechanism=landlock")) {
		t.Fatalf("out=%q", out)
	}
	if bytes.Count([]byte(out), []byte("WARNING:")) != 1 {
		t.Fatalf("expected one warning, got %q", out)
	}
	lastWarn = make(map[string]time.Time)
}
