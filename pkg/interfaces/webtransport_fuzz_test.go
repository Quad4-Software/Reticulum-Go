// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"strings"
	"testing"
)

func FuzzNormalizeWTPath(f *testing.F) {
	f.Add("")
	f.Add("/rns")
	f.Add("rns")
	f.Add("/rns/")
	f.Fuzz(func(t *testing.T, path string) {
		out := normalizeWTPath(path)
		if out == "" {
			t.Fatal("empty path")
		}
		if !strings.HasPrefix(out, "/") {
			t.Fatalf("path must start with /: %q", out)
		}
	})
}

func FuzzParseWTTransportMode(f *testing.F) {
	f.Add("")
	f.Add("datagram")
	f.Add("stream")
	f.Add("dual")
	f.Add("bogus")
	f.Fuzz(func(t *testing.T, mode string) {
		out, err := parseWTTransportMode(mode)
		if err != nil {
			return
		}
		switch out {
		case "datagram", "stream", "dual":
		default:
			t.Fatalf("unexpected mode %q from %q", out, mode)
		}
	})
}
