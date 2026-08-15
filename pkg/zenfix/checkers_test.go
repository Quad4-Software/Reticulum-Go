// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestRequestPathOnInterface(t *testing.T) {
	src := `package p
type T struct{}
func (t *T) RequestPath([]byte,string,[]byte,bool) error { return nil }
func f(t *T) { t.RequestPath(nil, "LoRa", nil, false) }
`
	fset := token.NewFileSet()
	file, err := parserParseFile(fset, "x.go", src)
	if err != nil {
		t.Fatal(err)
	}
	var call *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok && baseName(callName(c)) == "RequestPath" {
			call = c
		}
		return true
	})
	if !requestPathOnInterface(call) {
		t.Fatal("expected on_interface detection")
	}
}

func TestRecallBeforePathOrder(t *testing.T) {
	src := `package p

type identityPkg struct{}
func (identityPkg) Recall([]byte) (any, error) { return nil, nil }

var identity identityPkg

type Transport struct{}
func (t *Transport) AwaitPath(context.Context, []byte) error { return nil }

import "context"

func BadRecall(tr *Transport, dest []byte) {
	identity.Recall(dest)
	_ = tr.AwaitPath(context.Background(), dest)
}
`
	// fix import order
	src = `package p

import "context"

type identityPkg struct{}
func (identityPkg) Recall([]byte) (any, error) { return nil, nil }

var identity identityPkg

type Transport struct{}
func (t *Transport) AwaitPath(context.Context, []byte) error { return nil }

func BadRecall(tr *Transport, dest []byte) {
	identity.Recall(dest)
	_ = tr.AwaitPath(context.Background(), dest)
}
`
	fset := token.NewFileSet()
	findings, err := InspectGoFile(fset, "x.go", src)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range findings {
		if f.Rule == RuleRecallBeforePath {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected recall-before-path, got %v", findings)
	}
}

func TestCallBeforePathWait(t *testing.T) {
	if !callBeforePathWait(10, 0) {
		t.Fatal("zero path wait should flag recall")
	}
	if callBeforePathWait(20, 10) {
		t.Fatal("recall after path wait should be ok")
	}
}
