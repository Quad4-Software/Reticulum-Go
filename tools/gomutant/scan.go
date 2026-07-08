// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func swapOperator(op token.Token) (token.Token, bool) {
	switch op {
	case token.EQL:
		return token.NEQ, true
	case token.NEQ:
		return token.EQL, true
	case token.LSS:
		return token.GTR, true
	case token.GTR:
		return token.LSS, true
	case token.LEQ:
		return token.GEQ, true
	case token.GEQ:
		return token.LEQ, true
	case token.LAND:
		return token.LOR, true
	case token.LOR:
		return token.LAND, true
	case token.ADD:
		return token.SUB, true
	case token.SUB:
		return token.ADD, true
	default:
		return 0, false
	}
}

func operatorKind(op token.Token) mutantKind {
	switch op {
	case token.EQL, token.NEQ, token.LSS, token.GTR, token.LEQ, token.GEQ:
		return kindComparison
	case token.LAND, token.LOR:
		return kindLogical
	case token.ADD, token.SUB:
		return kindArithmetic
	default:
		return kindComparison
	}
}

func listPackageSources(pkg string) ([]string, error) {
	dir, err := packageDir(pkg)
	if err != nil {
		return nil, err
	}
	var files []string
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func scanPackage(pkg string, nextID int) ([]mutant, int, error) {
	files, err := listPackageSources(pkg)
	if err != nil {
		return nil, nextID, err
	}
	var out []mutant
	for _, file := range files {
		mutants, nextID, err := scanFile(pkg, file, nextID)
		if err != nil {
			return nil, nextID, err
		}
		out = append(out, mutants...)
	}
	return out, nextID, nil
}

func scanFile(pkg, path string, nextID int) ([]mutant, int, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nextID, err
	}
	if bytesContainsSkip(src) {
		return nil, nextID, nil
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, nextID, fmt.Errorf("parse %s: %w", path, err)
	}

	var out []mutant
	ast.Inspect(f, func(node ast.Node) bool {
		be, ok := node.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		newOp, ok := swapOperator(be.Op)
		if !ok {
			return true
		}
		oldStr := be.Op.String()
		newStr := newOp.String()
		if len(oldStr) != len(newStr) {
			return true
		}
		pos := fset.Position(be.OpPos)
		if pos.Offset+len(oldStr) > len(src) {
			return true
		}
		if string(src[pos.Offset:pos.Offset+len(oldStr)]) != oldStr {
			return true
		}
		mut := mutant{
			ID:     nextID,
			Pkg:    pkg,
			File:   path,
			Line:   pos.Line,
			Column: pos.Column,
			Kind:   operatorKind(be.Op),
			From:   oldStr,
			To:     newStr,
			Offset: pos.Offset,
			Length: len(oldStr),
			Source: append([]byte(nil), src...),
		}
		copy(mut.Source[pos.Offset:pos.Offset+len(newStr)], newStr)
		out = append(out, mut)
		nextID++
		return true
	})
	return out, nextID, nil
}

func bytesContainsSkip(src []byte) bool {
	return strings.Contains(string(src), "gomutant:skip-file")
}
