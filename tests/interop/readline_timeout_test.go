// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bufio"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestReadLineTimeoutNoConcurrentPanic(t *testing.T) {
	r, w := io.Pipe()
	br := bufio.NewReader(r)
	ctx := context.Background()

	// Time out before any newline arrives. Previously this left a stuck
	// ReadString goroutine that raced the next call and panics.
	if _, err := readLineTimeout(ctx, br, 20*time.Millisecond); err != context.DeadlineExceeded {
		t.Fatalf("first wait err=%v want deadline exceeded", err)
	}

	go func() {
		_, _ = io.WriteString(w, "hello\n")
		_ = w.Close()
	}()

	line, err := readLineTimeout(ctx, br, time.Second)
	if err != nil {
		t.Fatalf("second wait: %v", err)
	}
	if strings.TrimSpace(line) != "hello" {
		t.Fatalf("got %q", line)
	}
}
