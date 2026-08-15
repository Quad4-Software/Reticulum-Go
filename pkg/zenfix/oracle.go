// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"go/ast"
	"go/token"
)

// funcSpan maps a function name to its source line range.
type funcSpan struct {
	name  string
	start int
	end   int
}

func funcSpans(fset *token.FileSet, file *ast.File) []funcSpan {
	var spans []funcSpan
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		start := fset.Position(fn.Pos()).Line
		end := fset.Position(fn.End()).Line
		spans = append(spans, funcSpan{name: fn.Name.Name, start: start, end: end})
	}
	return spans
}

func rulesByFunc(fset *token.FileSet, file *ast.File, findings []Finding) map[string][]string {
	spans := funcSpans(fset, file)
	out := make(map[string][]string)
	for _, f := range findings {
		for _, sp := range spans {
			if f.Line >= sp.start && f.Line <= sp.end {
				out[sp.name] = append(out[sp.name], f.Rule)
			}
		}
	}
	return out
}

func ruleSet(rules []string) map[string]int {
	m := make(map[string]int, len(rules))
	for _, r := range rules {
		m[r]++
	}
	return m
}
