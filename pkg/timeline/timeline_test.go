// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package timeline

import "testing"

func TestSpecConstants(t *testing.T) {
	if SpecVersion < 1 {
		t.Fatal("spec version")
	}
	if StderrPrefix != "INTEROP_EVENT " {
		t.Fatalf("prefix %q", StderrPrefix)
	}
	if EventReady != "ready" || EventFail != "fail" || KindPath != "path" {
		t.Fatal("event or kind constants")
	}
}
