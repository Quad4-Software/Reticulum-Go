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
}

func TestCLIUnknownArg(t *testing.T) {
	var buf bytes.Buffer
	code := cli.Main([]string{"--bogus"}, cli.Options{Stdout: &buf, Stderr: &buf, VersionLine: versionLine()})
	if code != 2 {
		t.Fatalf("code=%d, want 2", code)
	}
}

func TestParseDaemonFlags(t *testing.T) {
	run, code := parseDaemonFlags(nil)
	if !run || code != 0 {
		t.Fatalf("empty args: run=%v code=%d", run, code)
	}
	run, code = parseDaemonFlags([]string{"--bogus"})
	if run || code != 2 {
		t.Fatalf("bogus: run=%v code=%d", run, code)
	}
}
