// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/term"
)

func TestConsoleUpdateStatusLiveTTY(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	old := opConsole
	opConsole = &operatorConsole{out: w, live: true}
	t.Cleanup(func() { opConsole = old })

	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "1")

	opConsole.updateStatus("line-one")
	opConsole.updateStatus("line-two")
	_ = w.Close()

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()
	if !strings.Contains(out, term.ProgressClear(w)) {
		t.Fatal("expected progress clear sequence in live output")
	}
	if !strings.HasSuffix(out, "line-two") {
		t.Fatalf("output=%q", out)
	}
}

func TestConsoleUpdateStatusNonLive(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})

	old := opConsole
	opConsole = &operatorConsole{out: w, live: false}
	t.Cleanup(func() { opConsole = old })

	opConsole.updateStatus("alpha")
	opConsole.updateStatus("beta")
	_ = w.Close()

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()
	if bytes.Count([]byte(out), []byte("alpha\n")) != 1 {
		t.Fatalf("expected alpha line, got %q", out)
	}
	if bytes.Count([]byte(out), []byte("beta\n")) != 1 {
		t.Fatalf("expected beta line, got %q", out)
	}
}

func TestStatusIntervalOverride(t *testing.T) {
	t.Setenv("GORRCD_STATUS_INTERVAL", "250ms")
	if got := statusInterval(true); got != 250*time.Millisecond {
		t.Fatalf("interval=%v", got)
	}
	t.Setenv("GORRCD_STATUS_INTERVAL", "nope")
	if got := statusInterval(true); got != liveStatusInterval {
		t.Fatalf("fallback interval=%v", got)
	}
}
