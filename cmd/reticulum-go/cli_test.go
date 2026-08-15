// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"bytes"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/cli"
)

func TestCLIVersion(t *testing.T) {
	var buf bytes.Buffer
	code := cli.Main([]string{"--version"}, cli.Options{
		Stdout:      &buf,
		Stderr:      &buf,
		VersionLine: versionLine(),
	})
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(buf.String(), "reticulum-go") {
		t.Fatalf("version output: %q", buf.String())
	}
}

func TestCLIHelp(t *testing.T) {
	var buf bytes.Buffer
	code := cli.Main([]string{"--help"}, cli.Options{Stdout: &buf, Stderr: &buf, VersionLine: versionLine()})
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(buf.String(), "status") {
		t.Fatalf("help: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "debug") {
		t.Fatalf("help missing debug: %q", buf.String())
	}
}

func TestCLIUnknownArg(t *testing.T) {
	var buf bytes.Buffer
	code := cli.Main([]string{"--bogus"}, cli.Options{
		Stdout:      &buf,
		Stderr:      &buf,
		VersionLine: versionLine(),
		RunDaemon:   runDaemonCLI,
	})
	if code != 2 {
		t.Fatalf("code=%d, want 2", code)
	}
}

func TestCLIUnknownCommand(t *testing.T) {
	var buf bytes.Buffer
	code := cli.Main([]string{"not-a-command"}, cli.Options{Stdout: &buf, Stderr: &buf, VersionLine: versionLine()})
	if code != 2 {
		t.Fatalf("code=%d, want 2", code)
	}
}

func TestParseDaemonFlags(t *testing.T) {
	opts, run, code := parseDaemonFlags(nil)
	if !run || code != 0 {
		t.Fatalf("empty args: run=%v code=%d", run, code)
	}
	if opts.DebugLevel != -1 {
		t.Fatalf("debug default: %d", opts.DebugLevel)
	}
	opts, run, code = parseDaemonFlags([]string{"--bogus"})
	if run || code != 2 {
		t.Fatalf("bogus: run=%v code=%d", run, code)
	}
	opts, run, code = parseDaemonFlags([]string{"-config", "/tmp/x", "-debug", "5"})
	if !run || code != 0 {
		t.Fatalf("config/debug: run=%v code=%d", run, code)
	}
	if opts.ConfigPath != "/tmp/x" || opts.DebugLevel != 5 {
		t.Fatalf("opts=%+v", opts)
	}
}
