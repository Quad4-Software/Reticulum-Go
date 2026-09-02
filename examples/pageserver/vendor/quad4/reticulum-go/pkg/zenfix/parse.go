// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"go/ast"
	"go/parser"
	"go/token"
)

func parserParseFile(fset *token.FileSet, path string, src any) (*ast.File, error) {
	return parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
}
