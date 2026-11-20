// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package common

import "testing"

// TestNormalizeWireVersion covers the TRANSPORT_VERS value rules: Python
// version_tuple requires each leading component to start with digits, so
// release-style v prefixes strip and unparseable strings fall back to a
// dotted dev version rather than provoking a protocol violation.
func TestNormalizeWireVersion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"1.6.0", "1.6.0"},
		{"v1.6.0", "1.6.0"},
		{"V1.6.0", "1.6.0"},
		{"1.6.0-3-gabc123", "1.6.0-3-gabc123"},
		{"dev", "0.0.0-dev"},
		{"", "0.0.0-dev"},
		{"(devel)", "0.0.0-dev"},
	}
	for _, tc := range cases {
		if got := normalizeWireVersion(tc.in); got != tc.want {
			t.Fatalf("normalizeWireVersion(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
