// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"strings"
	"testing"
)

func FuzzParseRNSTXT(f *testing.F) {
	f.Add("rns=udp://127.0.0.1:4242")
	f.Add("rns proto=udp host=10.0.0.1 port=9999")
	f.Add("rns=tcp://[::1]:4242")
	f.Add("unrelated")
	f.Add("")
	f.Add("rns=udp://")
	f.Add("rns host=only")
	f.Add("rns=udp://bad-port:xyz")
	f.Fuzz(func(t *testing.T, txt string) {
		ep, ok := ParseRNSTXT(txt)
		if !ok {
			return
		}
		if ep.Host == "" || ep.Port <= 0 {
			t.Fatalf("accepted invalid endpoint: %+v from %q", ep, txt)
		}
		if ep.Proto == "" {
			t.Fatalf("empty proto for %q", txt)
		}
		if strings.ContainsAny(ep.Host, " \t\n") {
			t.Fatalf("host has whitespace: %q", ep.Host)
		}
	})
}
