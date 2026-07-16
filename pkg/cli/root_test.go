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
	help := out.String()
	if !strings.Contains(help, "pageserver") {
		t.Fatalf("help missing pageserver: %s", help)
	}
	if !strings.Contains(help, "reticulum-go x") {
		t.Fatalf("help missing x: %s", help)
	}
	if !strings.Contains(help, "self-check") {
		t.Fatalf("help missing self-check: %s", help)
	}
}

func TestResolveCommandSelfCheck(t *testing.T) {
	cmd, rest, ok := resolveCommand("reticulum-go", []string{"self-check", "--json"})
	if !ok || cmd != CmdSelfCheck {
		t.Fatalf("got ok=%v cmd=%q", ok, cmd)
	}
	if len(rest) != 1 || rest[0] != "--json" {
		t.Fatalf("rest=%v", rest)
	}
	cmd, _, ok = resolveCommand("rgoselfcheck", nil)
	if !ok || cmd != CmdSelfCheck {
		t.Fatalf("alias ok=%v cmd=%q", ok, cmd)
	}
}

func TestResolveCommandX(t *testing.T) {
	cmd, _, ok := resolveCommand("reticulum-go", []string{"x", "-l"})
	if !ok || cmd != CmdX {
		t.Fatalf("subcommand ok=%v cmd=%q", ok, cmd)
	}
	cmd, _, ok = resolveCommand("/usr/bin/rnx", []string{"-p"})
	if !ok || cmd != CmdX {
		t.Fatalf("rnx alias ok=%v cmd=%q", ok, cmd)
	}
	cmd, _, ok = resolveCommand("rgox", nil)
	if !ok || cmd != CmdX {
		t.Fatalf("rgox alias ok=%v cmd=%q", ok, cmd)
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

func TestMainDaemonFlags(t *testing.T) {
	var got []string
	code := Main([]string{"--config", "/tmp/x"}, Options{
		RunDaemon: func(args []string) int {
			got = append([]string(nil), args...)
			return 0
		},
	})
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if len(got) != 2 || got[0] != "--config" || got[1] != "/tmp/x" {
		t.Fatalf("args=%v", got)
	}
}

func TestMainUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	code := Main([]string{"not-a-command"}, Options{Stdout: &out, Stderr: &out, VersionLine: "test"})
	if code != 2 {
		t.Fatalf("code=%d", code)
	}
}
