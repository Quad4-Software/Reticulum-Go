// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import "testing"

// skipHeavyLinkTestsIfShort skips interop-style tests that spin links, watchdogs,
// and multi-second transfers. Use `go test ./pkg/link/` without -short for the
// full suite; CI and local quick checks should use -short.
func skipHeavyLinkTestsIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("heavy link interop test (run without -short for full coverage)")
	}
}
