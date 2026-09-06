// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"os"
	"path/filepath"
	"testing"

	"go/token"
)

func TestRunOnTempModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module zentest\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `package main
type T struct{}
func (t *T) RequestPath([]byte,string,[]byte,bool) error { return nil }
func main() { for { var t T; t.RequestPath(nil,"",nil,false) } }
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(Options{Dir: dir, Patterns: []string{"."}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Errors == 0 {
		t.Fatal("expected error finding")
	}
}

func TestInspectGoFileAPI(t *testing.T) {
	fset := token.NewFileSet()
	findings, err := InspectGoFile(fset, "x.go", `package p
type T struct{}
func (t *T) RequestPath([]byte,string,[]byte,bool) error { return nil }
func f() { for { var t T; t.RequestPath(nil,"",nil,false) } }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings from InspectGoFile")
	}
}

func TestOraclePythonBad(t *testing.T) {
	path := filepath.Join("testdata", "module", "python", "bad.py")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	findings := scanPython(path, string(b))
	rules := ruleSet(nil)
	for _, f := range findings {
		rules[f.Rule]++
	}
	for _, id := range []string{"zen/python-path-spin", "zen/python-requestpath-loop"} {
		if rules[id] == 0 {
			t.Fatalf("missing %s in bad.py, got %v", id, rules)
		}
	}
	goodPath := filepath.Join("testdata", "module", "python", "good.py")
	gb, err := os.ReadFile(goodPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range scanPython(goodPath, string(gb)) {
		t.Fatalf("good.py flagged: %+v", f)
	}
}
