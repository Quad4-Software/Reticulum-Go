// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go/parser"
	"go/token"
)

func TestInspectBadPatterns(t *testing.T) {
	path := filepath.Join("testdata", "bad.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	findings := inspectFile(fset, path, file)
	rules := make(map[string]int)
	for _, f := range findings {
		rules[f.Rule]++
	}
	want := []string{
		"zen/requestpath-loop",
		"zen/establish-loop",
		"zen/requestpath-ignored-error",
		"zen/establish-no-await",
		"zen/fixed-15s-timeout",
	}
	for _, rule := range want {
		if rules[rule] == 0 {
			t.Fatalf("missing rule %s, got %v", rule, rules)
		}
	}
}

func TestScanPython(t *testing.T) {
	path := filepath.Join("testdata", "bad.py")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	findings := scanPython(path, string(b))
	if len(findings) < 2 {
		t.Fatalf("expected python findings, got %d", len(findings))
	}
}

func TestApplyFix(t *testing.T) {
	path := filepath.Join("testdata", "fix.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	findings := inspectFile(fset, path, f)
	var fixable []Finding
	for _, finding := range findings {
		if finding.Fix != nil {
			fixable = append(fixable, finding)
		}
	}
	if len(fixable) == 0 {
		t.Fatal("expected fixable finding")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "fix.go")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, b, 0o644); err != nil {
		t.Fatal(err)
	}
	for i := range fixable {
		fixable[i].File = file
	}
	n, err := applyFixes(fixable)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("fixed %d, want 1", n)
	}
	after, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "if err := tr.RequestPath") {
		t.Fatalf("fix not applied: %s", after)
	}
}
