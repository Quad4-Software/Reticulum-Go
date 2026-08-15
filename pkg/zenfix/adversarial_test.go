// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"strings"
	"testing"

	"go/parser"
	"go/token"
)

func TestAdversarialNoPanicOnMalformedGo(t *testing.T) {
	cases := []string{
		"package p\n",
		"package p\n// only comments\n",
		"package p\nfunc f() {",
		"package p\nfunc f() { /* unclosed",
	}
	for _, src := range cases {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "x.go", src, 0)
		if err != nil {
			continue
		}
		_ = inspectFile(fset, "x.go", file)
	}
}

func TestAdversarialDeeplyNestedLoops(t *testing.T) {
	src := `package p
type T struct{}
func (t *T) RequestPath([]byte,string,[]byte,bool) error { return nil }
func deep() {
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			for k := 0; k < 2; k++ {
				var tr T
				tr.RequestPath(nil, "", nil, false)
			}
		}
	}
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "deep.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	findings := inspectFile(fset, "deep.go", file)
	if len(findings) == 0 {
		t.Fatalf("expected nested loop finding, got %d", len(findings))
	}
}

func TestAdversarialPythonEmptyAndBinary(t *testing.T) {
	if findings := scanPython("x.py", ""); len(findings) != 0 {
		t.Fatalf("empty python: %v", findings)
	}
	blob := strings.Repeat("x\n", 5000) + "while not Transport.has_path(d):\n pass\n"
	findings := scanPython("blob.py", blob)
	if len(findings) == 0 {
		t.Fatal("expected finding in blob")
	}
}

func TestDedupeFindings(t *testing.T) {
	in := []Finding{
		{Rule: "zen/a", File: "f.go", Line: 1, Col: 1},
		{Rule: "zen/a", File: "f.go", Line: 1, Col: 1},
		{Rule: "zen/b", File: "f.go", Line: 2, Col: 1},
	}
	out := dedupeFindings(in)
	if len(out) != 2 {
		t.Fatalf("dedupe len = %d", len(out))
	}
}

func TestAllRulesCatalogNonEmpty(t *testing.T) {
	if len(AllRules) < 10 {
		t.Fatalf("rules catalog too small: %d", len(AllRules))
	}
	seen := make(map[string]struct{})
	for _, r := range AllRules {
		if r.ID == "" {
			t.Fatal("empty rule id")
		}
		if _, ok := seen[r.ID]; ok {
			t.Fatalf("duplicate rule %s", r.ID)
		}
		seen[r.ID] = struct{}{}
	}
}
