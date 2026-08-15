// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"strings"
	"testing"
)

// FuzzParseByteSize ensures byte-size parsing never panics and rejects
// empty or clearly invalid inputs with an error.
func FuzzParseByteSize(f *testing.F) {
	f.Add("")
	f.Add("0")
	f.Add("64K")
	f.Add("1M")
	f.Add("2G")
	f.Add("-1")
	f.Add("abc")
	f.Add("999999999999999999999")

	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<12 {
			t.Skip()
		}
		n, err := ParseByteSize(s)
		if strings.TrimSpace(s) == "" {
			if err == nil {
				t.Fatal("empty must error")
			}
			return
		}
		if err != nil {
			return
		}
		if n < 0 {
			t.Fatalf("negative size %d from %q", n, s)
		}
	})
}

// FuzzParseInterfaceMode covers mode string aliases without panicking.
func FuzzParseInterfaceMode(f *testing.F) {
	f.Add("")
	f.Add("full")
	f.Add("gateway")
	f.Add("access_point")
	f.Add("roaming")
	f.Add("unknown")

	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<10 {
			t.Skip()
		}
		_ = ParseInterfaceMode(s)
	})
}
