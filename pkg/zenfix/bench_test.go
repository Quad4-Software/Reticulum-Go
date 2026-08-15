// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"os"
	"path/filepath"
	"testing"

	"go/parser"
	"go/token"
)

func benchParseInspect(b *testing.B, path string) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		inspectFile(fset, path, file)
	}
}

func BenchmarkInspectBadGo(b *testing.B) {
	benchParseInspect(b, filepath.Join("testdata", "bad.go"))
}

func BenchmarkInspectGoodGo(b *testing.B) {
	benchParseInspect(b, filepath.Join("testdata", "module", "good", "good.go"))
}

func BenchmarkInspectBadModule(b *testing.B) {
	benchParseInspect(b, filepath.Join("testdata", "module", "bad", "bad.go"))
}

func BenchmarkScanPythonBad(b *testing.B) {
	path := filepath.Join("testdata", "bad.py")
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	src := string(data)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scanPython(path, src)
	}
}

func BenchmarkScanPythonPatterns(b *testing.B) {
	path := filepath.Join("testdata", "module", "python", "rns_patterns.py")
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	src := string(data)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scanPython(path, src)
	}
}

func BenchmarkRunTestModule(b *testing.B) {
	dir := filepath.Join("testdata", "module")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Run(Options{Dir: dir, Patterns: []string{"./..."}}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRunTestModulePython(b *testing.B) {
	dir := filepath.Join("testdata", "module")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Run(Options{Dir: dir, Patterns: []string{"./..."}, Python: true}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRunTransportPkg(b *testing.B) {
	root, err := repoRootBench(b)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Run(Options{
			Dir:      root,
			Patterns: []string{"./pkg/transport"},
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func repoRootBench(b *testing.B) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			b.Fatal("go.mod not found")
		}
		dir = parent
	}
}
