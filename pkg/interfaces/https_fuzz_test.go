// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"strings"
	"testing"
	"time"
)

func FuzzNormalizeHTTPSPath(f *testing.F) {
	f.Add("")
	f.Add("/rns")
	f.Add("rns")
	f.Add("/rns/")
	f.Add("//evil")
	f.Add("/a/b/c")
	f.Fuzz(func(t *testing.T, path string) {
		out := normalizeHTTPSPath(path)
		if out == "" {
			t.Fatal("empty path")
		}
		if !strings.HasPrefix(out, "/") {
			t.Fatalf("path must start with /: %q", out)
		}
		if strings.HasSuffix(out, "/") && out != "/" {
			t.Fatalf("trailing slash not stripped: %q", out)
		}
	})
}

func FuzzNormalizeHTTPSLongPoll(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(-1))
	f.Add(int64(25))
	f.Add(int64(time.Hour))
	f.Fuzz(func(t *testing.T, nsec int64) {
		d := normalizeHTTPSLongPoll(time.Duration(nsec))
		if d <= 0 {
			t.Fatalf("non-positive long poll: %v", d)
		}
	})
}
