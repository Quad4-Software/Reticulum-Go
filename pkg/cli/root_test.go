// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestResolveCommandSubcommand(t *testing.T) {
	cmd, rest, ok := resolveCommand("reticulum-go", []string{"status", "-json"})
	if !ok || cmd != CmdStatus {
		t.Fatalf("got ok=%v cmd=%q", ok, cmd)
	}
	if len(rest) != 1 || rest[0] != "-json" {
		t.Fatalf("rest=%v", rest)
	}
}

func TestResolveCommandArgv0Alias(t *testing.T) {
	cmd, rest, ok := resolveCommand("/usr/local/bin/rgostatus", []string{"-a"})
	if !ok || cmd != CmdStatus {
		t.Fatalf("got ok=%v cmd=%q", ok, cmd)
	}
	if len(rest) != 1 || rest[0] != "-a" {
		t.Fatalf("rest=%v", rest)
	}
}

func TestMainHelp(t *testing.T) {
	var out bytes.Buffer
	code := Main([]string{"--help"}, Options{Stdout: &out, Stderr: &out, VersionLine: "test"})
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out.String(), "pageserver") {
		t.Fatalf("help missing pageserver: %s", out.String())
	}
}

func TestMainVersion(t *testing.T) {
	var out bytes.Buffer
	code := Main([]string{"--version"}, Options{Stdout: &out, Stderr: &out, VersionLine: "reticulum-go test"})
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out.String(), "reticulum-go test") {
		t.Fatalf("version: %q", out.String())
	}
}

func TestMainDaemonDefault(t *testing.T) {
	called := false
	code := Main(nil, Options{
		RunDaemon: func(args []string) int {
			called = true
			return 0
		},
	})
	if code != 0 || !called {
		t.Fatalf("code=%d called=%v", code, called)
	}
}
