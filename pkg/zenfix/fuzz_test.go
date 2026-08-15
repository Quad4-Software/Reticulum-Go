// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"unicode/utf8"
)

func FuzzScanPython(f *testing.F) {
	f.Add([]byte("while not Transport.has_path(x):\n  time.sleep(1)\n"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, data []byte) {
		if !utf8.Valid(data) {
			return
		}
		_ = scanPython("fuzz.py", string(data))
	})
}

func FuzzInspectGoFile(f *testing.F) {
	seed := []byte(`package p
func f() { for { x() } }
func x() {}
`)
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64*1024 {
			return
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "fuzz.go", data, parser.AllErrors)
		if err != nil {
			return
		}
		_ = inspectFile(fset, "fuzz.go", file)
	})
}

func FuzzMatchFifteenSeconds(f *testing.F) {
	f.Add(int64(15))
	f.Add(int64(0))
	f.Fuzz(func(t *testing.T, n int64) {
		if n < 0 || n > 1000 {
			return
		}
		src := "package p\nimport \"time\"\nfunc f() { time.Sleep(time.Duration(" + itoa(n) + ") * time.Second) }\n"
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "t.go", src, 0)
		if err != nil {
			return
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				_ = fifteenSecondArg(call)
			}
			return true
		})
	})
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func FuzzDedupeFindings(f *testing.F) {
	f.Add(uint8(3), uint8(1))
	f.Fuzz(func(t *testing.T, n, rule byte) {
		if n == 0 {
			n = 1
		}
		var in []Finding
		for i := range int(n) {
			in = append(in, Finding{
				Rule: "zen/r" + string(rune('a'+rule%5)),
				File: "f.go",
				Line: i + 1,
				Col:  1,
			})
		}
		out := dedupeFindings(in)
		if len(out) > len(in) {
			t.Fatal("dedupe grew slice")
		}
	})
}
