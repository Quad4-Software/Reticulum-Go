// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
)

func TestWarnFootgunCooldown(t *testing.T) {
	var logs bytes.Buffer
	debug.SetExtraWriter(&logs)
	t.Cleanup(func() { debug.SetExtraWriter(nil) })

	l := &Link{}
	l.warnFootgun("first misuse", "k", 1)
	l.warnFootgun("second misuse", "k", 2)

	out := logs.String()
	if !strings.Contains(out, "first misuse") {
		t.Fatalf("first warn missing:\n%s", out)
	}
	if strings.Contains(out, "second misuse") {
		t.Fatalf("second warn inside cooldown should be suppressed:\n%s", out)
	}

	// After the cooldown passes the next warn fires again.
	l.footgunWarnAt.Store(time.Now().Add(-2 * time.Minute).UnixNano())
	l.warnFootgun("third misuse", "k", 3)
	if !strings.Contains(logs.String(), "third misuse") {
		t.Fatalf("warn after cooldown missing:\n%s", logs.String())
	}
}

func TestWarnFootgunConcurrentOnce(t *testing.T) {
	var logs bytes.Buffer
	debug.SetExtraWriter(&logs)
	t.Cleanup(func() { debug.SetExtraWriter(nil) })

	l := &Link{}
	done := make(chan struct{}, 32)
	for range 32 {
		go func() {
			defer func() { done <- struct{}{} }()
			l.warnFootgun("concurrent misuse")
		}()
	}
	for range 32 {
		<-done
	}
	if got := strings.Count(logs.String(), "concurrent misuse"); got != 1 {
		t.Fatalf("concurrent warns = %d, want exactly 1", got)
	}
}
