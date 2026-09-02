// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"go/ast"
	"go/token"
	"strings"
)

func requestPathOnInterface(call *ast.CallExpr) bool {
	if call == nil || !isPathRequestBase(baseName(callName(call))) {
		return false
	}
	if len(call.Args) >= 2 {
		return argLooksNonEmpty(call.Args[1])
	}
	return false
}

func argLooksNonEmpty(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return true
		}
		return e.Value != `""` && e.Value != ``
	case *ast.Ident:
		return e.Name != "" && e.Name != "_"
	default:
		return true
	}
}

func callBeforePathWait(callPos, pathWaitPos token.Pos) bool {
	if pathWaitPos == 0 {
		return true
	}
	return callPos < pathWaitPos
}

func lineHasAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
