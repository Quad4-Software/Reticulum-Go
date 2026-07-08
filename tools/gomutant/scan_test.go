// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func TestScanFileFindsComparisonMutants(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	const src = `package sample

func Positive(n int) bool {
	return n > 0
}
`
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	mutants, _, err := scanFile("./sample", path, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(mutants) != 1 {
		t.Fatalf("mutants=%d", len(mutants))
	}
	if mutants[0].From != ">" || mutants[0].To != "<" {
		t.Fatalf("swap=%s->%s", mutants[0].From, mutants[0].To)
	}
	if string(mutants[0].Source[mutants[0].Offset:mutants[0].Offset+1]) != "<" {
		t.Fatal("mutated source not applied")
	}
}

func TestSwapOperatorPairs(t *testing.T) {
	pairs := []struct{ from, to string }{
		{"==", "!="},
		{"!=", "=="},
		{"&&", "||"},
		{"+", "-"},
	}
	for _, p := range pairs {
		got, ok := swapOperator(tokenFromString(p.from))
		if !ok {
			t.Fatalf("no swap for %s", p.from)
		}
		if got.String() != p.to {
			t.Fatalf("%s -> %s", p.from, got.String())
		}
	}
}

func tokenFromString(s string) token.Token {
	switch s {
	case "==":
		return token.EQL
	case "!=":
		return token.NEQ
	case "&&":
		return token.LAND
	case "+":
		return token.ADD
	default:
		panic("unknown token " + s)
	}
}
