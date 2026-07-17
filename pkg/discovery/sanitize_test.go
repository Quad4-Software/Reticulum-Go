// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"strings"
	"testing"
)

func TestSanitizeNameMatchesPythonStyle(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"  Hello World  ", "Hello World"},
		{"***Name***", "Name"},
		{"café-node", "caf-node"},
		{"a\nb\rc", "abc"},
		{"(((bad", "bad"},
		{"ok)", "ok)"},
	}
	for _, tc := range cases {
		got := sanitize(tc.in)
		if got != tc.want {
			t.Fatalf("sanitize(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
	got := sanitize(strings.Repeat("a", 300))
	if len(got) != 255 {
		t.Fatalf("sanitize length cap: got %d want 255", len(got))
	}
}
