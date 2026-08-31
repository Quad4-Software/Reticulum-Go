// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzLoadConfigExploratory feeds arbitrary file contents through LoadConfig.
// The parser must not panic, and reserved section names must never appear
// as interface entries.
func FuzzLoadConfigExploratory(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("[reticulum]\nenable_transport = yes\n"))
	f.Add([]byte("[reticulum]\n[[Default Interface]]\ntype = UDPInterface\n"))
	f.Add([]byte("[interfaces]\n[[Bad]]\ntype = UDPInterface\nlisten_ip = 127.0.0.1\n"))
	f.Add([]byte("[logging]\nloglevel = not-a-number\n"))
	f.Add([]byte("[[[[too deep]]]]\n"))
	f.Add([]byte{0xef, 0xbb, 0xbf, '[', 'r', 'e', 't', 'i', 'c', 'u', 'l', 'u', 'm', ']', '\n'})
	f.Add([]byte("key = value\n# comment\n"))
	f.Add([]byte{0xff, 0xfe, 0x00})

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<16 {
			t.Skip()
		}
		path := filepath.Join(t.TempDir(), "config")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadConfig(path)
		if err != nil {
			return
		}
		if cfg == nil {
			t.Fatal("nil config without error")
		}
		if cfg.Interfaces == nil {
			t.Fatal("Interfaces map must be non-nil")
		}
		for name := range cfg.Interfaces {
			lower := strings.ToLower(name)
			switch lower {
			case "reticulum", "logging", "interfaces":
				t.Fatalf("reserved section leaked as interface %q", name)
			}
		}
	})
}

func TestClassifySectionExploratory(t *testing.T) {
	if got := classifySection("reticulum", 1); got != sectionReticulum {
		t.Fatalf("got %q", got)
	}
	if got := classifySection("logging", 1); got != sectionLogging {
		t.Fatalf("got %q", got)
	}
	if got := classifySection("interfaces", 1); got != sectionInterfaces {
		t.Fatalf("got %q", got)
	}
	if got := classifySection("UDP", 1); got != sectionUnknown {
		t.Fatalf("unknown depth1 got %q", got)
	}
	if got := classifySection("anything", 2); got != sectionInterface {
		t.Fatalf("depth2 got %q", got)
	}
	if got := classifySection("sub", 3); got != sectionSubInterface {
		t.Fatalf("depth3 got %q", got)
	}
}

func TestStripInlineCommentExploratory(t *testing.T) {
	if got := stripInlineComment("yes #x"); got != "yes" {
		t.Fatalf("comment strip got %q", got)
	}
	if got := stripInlineComment("/tmp/#keep"); got != "/tmp/#keep" {
		t.Fatalf("path hash got %q", got)
	}
	if got := stripInlineComment("plain"); got != "plain" {
		t.Fatalf("plain got %q", got)
	}
}
