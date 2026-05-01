// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
)

func muteDebugLogsForBenchmark(b *testing.B) {
	b.Helper()
	if debug.GetDebugLevel() > debug.DebugCritical {
		debug.SetDebugLevel(debug.DebugCritical)
	}
}
