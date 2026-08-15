// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptanceCorpusGo(t *testing.T) {
	dir := filepath.Join("testdata", "module")
	res, err := Run(Options{
		Dir:      dir,
		Patterns: []string{"./..."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Errors == 0 {
		t.Fatalf("expected corpus errors, got summary %+v", res.Summary)
	}
	rules := ruleSet(nil)
	for _, f := range res.Findings {
		rules[f.Rule]++
	}
	for _, id := range []string{
		"zen/requestpath-loop",
		"zen/haspath-loop",
		"zen/newlink-loop",
		"zen/link-not-active",
	} {
		if rules[id] == 0 {
			t.Fatalf("acceptance missing %s in corpus scan", id)
		}
	}
}

func TestAcceptanceCorpusGoodClean(t *testing.T) {
	dir := filepath.Join("testdata", "module")
	res, err := Run(Options{
		Dir:      dir,
		Patterns: []string{"./good"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Findings {
		t.Fatalf("good package finding: %+v", f)
	}
}

func TestAcceptanceCorpusPython(t *testing.T) {
	dir := filepath.Join("testdata", "module")
	resAll, err := Run(Options{
		Dir:    dir,
		Python: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pyRules := ruleSet(nil)
	for _, f := range resAll.Findings {
		if filepath.Base(filepath.Dir(f.File)) == "python" {
			pyRules[f.Rule]++
		}
	}
	if pyRules["zen/python-path-spin"] == 0 {
		t.Fatal("expected python path spin in bad.py scan")
	}
	for _, f := range resAll.Findings {
		if strings.HasSuffix(f.File, "good.py") {
			t.Fatalf("clean python file flagged: %+v", f)
		}
	}
}

func TestAcceptanceFixRoundTrip(t *testing.T) {
	src := "package p\n\nfunc BadIgnoredError(tr *Transport, dest []byte) error {\n\ttr.RequestPath(dest, \"\", nil, false)\n\treturn nil\n}\n\ntype Transport struct{}\nfunc (t *Transport) RequestPath([]byte, string, []byte, bool) error { return nil }\n"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixrt\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fix.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(Options{
		Dir:      dir,
		Patterns: []string{"."},
		Fix:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Fixed == 0 {
		t.Fatal("expected fix applied")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "if err := tr.RequestPath") {
		t.Fatalf("fix missing: %s", after)
	}
}
