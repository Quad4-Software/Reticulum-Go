// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sandbox

import (
	"fmt"
	"os"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/term"
)

var (
	warnMu   sync.Mutex
	lastWarn = make(map[string]time.Time)
)

const softWarnInterval = 30 * time.Second

// warnSoftUnavailable prints a rate-limited stdout warning when a sandbox
// mechanism soft-fails but the daemon continues.
func warnSoftUnavailable(mechanism, reason string) {
	if mechanism == "" {
		mechanism = "unknown"
	}
	now := time.Now()
	warnMu.Lock()
	if t := lastWarn[mechanism]; !t.IsZero() && now.Sub(t) < softWarnInterval {
		warnMu.Unlock()
		return
	}
	lastWarn[mechanism] = now
	warnMu.Unlock()
	_, _ = fmt.Fprintf(os.Stdout, "%s sandbox soft-unavailable mechanism=%s reason=%s\n",
		term.YellowW(os.Stdout, "WARNING:"), mechanism, reason)
}
