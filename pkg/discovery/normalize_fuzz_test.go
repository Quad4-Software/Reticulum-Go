// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"testing"
)

// FuzzNormalizeIfaceType keeps type alias normalization panic-free and
// deterministic for arbitrary strings.
func FuzzNormalizeIfaceType(f *testing.F) {
	f.Add("")
	f.Add("tcpserver")
	f.Add("TCPServerInterface")
	f.Add("backbone")
	f.Add("not-a-type")
	f.Add(string(make([]byte, 512)))

	f.Fuzz(func(t *testing.T, in string) {
		if len(in) > 1<<12 {
			t.Skip()
		}
		got := normalizeIfaceType(in)
		again := normalizeIfaceType(got)
		if got != again && normalizeIfaceType(again) != again {
			t.Fatalf("normalize not stable: %q -> %q -> %q", in, got, again)
		}
		_ = isDiscoverableType(in)
	})
}
