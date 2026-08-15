// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunZenOnCorpus(t *testing.T) {
	dir := filepath.Join("..", "zenfix", "testdata", "module")
	var stdout, stderr bytes.Buffer
	code := RunZen([]string{"-C", dir, "./bad"}, Options{Stdout: &stdout, Stderr: &stderr})
	if code == 0 {
		t.Fatalf("expected findings, stdout=%s", stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "zen/requestpath-loop") {
		t.Fatalf("missing rule in output: %s", out)
	}
	if !strings.Contains(out, "why:") {
		t.Fatalf("missing why in output: %s", out)
	}
	if !strings.Contains(out, "see:") {
		t.Fatalf("missing refs in output: %s", out)
	}
}

func TestRunZenGoodClean(t *testing.T) {
	dir := filepath.Join("..", "zenfix", "testdata", "module")
	var stdout, stderr bytes.Buffer
	code := RunZen([]string{"-C", dir, "./good"}, Options{Stdout: &stdout, Stderr: &stderr})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "no zen issues found") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunZenJSON(t *testing.T) {
	dir := filepath.Join("..", "zenfix", "testdata", "module")
	var stdout, stderr bytes.Buffer
	code := RunZen([]string{"-C", dir, "-json", "./bad"}, Options{Stdout: &stdout, Stderr: &stderr})
	if code == 0 {
		t.Fatal("expected non-zero for bad package")
	}
	out := stdout.String()
	for _, key := range []string{`"rule"`, `"why"`, `"refs"`, `"hint"`} {
		if !strings.Contains(out, key) {
			t.Fatalf("json missing %s: %s", key, out)
		}
	}
}

func TestMainZenHelp(t *testing.T) {
	var out bytes.Buffer
	code := Main([]string{"zen", "-h"}, Options{Stdout: &out, Stderr: &out, VersionLine: "test"})
	if code != 0 && code != 2 {
		t.Fatalf("zen -h code=%d", code)
	}
	if !strings.Contains(out.String(), "-fix") {
		t.Fatalf("zen help missing -fix: %s", out.String())
	}
	if !strings.Contains(out.String(), "-list-rules") {
		t.Fatalf("zen help missing -list-rules: %s", out.String())
	}
}

func TestRunZenListRules(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunZen([]string{"-list-rules"}, Options{Stdout: &stdout, Stderr: &stderr})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "zen/requestpath-loop") {
		t.Fatalf("missing rules: %s", out)
	}
	if !strings.Contains(out, "why:") {
		t.Fatalf("missing why in list: %s", out)
	}
}

func TestRunZenPython(t *testing.T) {
	dir := filepath.Join("..", "zenfix", "testdata", "module")
	var stdout, stderr bytes.Buffer
	code := RunZen([]string{"-C", dir, "-python"}, Options{Stdout: &stdout, Stderr: &stderr})
	if code == 0 {
		t.Fatalf("expected python findings stdout=%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "zen/python-path-spin") {
		t.Fatalf("missing python rule: %s", stdout.String())
	}
}
