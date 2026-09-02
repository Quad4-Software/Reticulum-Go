// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
)

// Result holds findings and fix stats.
type Result struct {
	Findings []Finding
	Summary  Summary
}

// Run analyzes packages and optionally applies fixes.
func Run(opts Options) (Result, error) {
	if len(opts.Patterns) == 0 {
		opts.Patterns = []string{"./..."}
	}
	dir := opts.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return Result{}, err
		}
	}

	var findings []Finding
	goFindings, err := analyzeGo(opts, dir)
	if err != nil {
		return Result{}, err
	}
	findings = append(findings, goFindings...)

	if opts.Python {
		pyFindings, err := analyzePython(dir)
		if err != nil {
			return Result{}, err
		}
		findings = append(findings, pyFindings...)
	}

	findings = dedupeFindings(findings)
	res := Result{Findings: findings, Summary: summarize(findings)}
	if opts.Fix {
		fixed, err := applyFixes(findings)
		if err != nil {
			return res, err
		}
		res.Summary.Fixed = fixed
	}
	return res, nil
}

// InspectGoFile parses one Go file and returns findings. Used by tests.
func InspectGoFile(fset *token.FileSet, path string, src any) ([]Finding, error) {
	file, err := parserParseFile(fset, path, src)
	if err != nil {
		return nil, err
	}
	return dedupeFindings(inspectFile(fset, path, file)), nil
}

func analyzeGo(opts Options, dir string) ([]Finding, error) {
	files, err := listGoFiles(dir, opts.Patterns, opts.Tests)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, path := range files {
		fset := token.NewFileSet()
		file, err := parserParseFile(fset, path, nil)
		if err != nil {
			continue
		}
		out = append(out, inspectFile(fset, path, file)...)
	}
	return dedupeFindings(out), nil
}

type visitor struct {
	fset           *token.FileSet
	file           string
	src            []byte
	findings       []Finding
	loopDepth      int
	fn             *ast.FuncDecl
	sawAwait       bool
	sawLinkCB      bool
	establishCount int
	newLinkCount   int
	pathWaitPos    token.Pos
}

func inspectFile(fset *token.FileSet, path string, file *ast.File) []Finding {
	v := &visitor{fset: fset, file: path}
	ast.Inspect(file, v.visit)
	return v.findings
}

func (v *visitor) visit(n ast.Node) bool {
	switch node := n.(type) {
	case nil:
		return false
	case *ast.FuncDecl:
		v.fn = node
		v.sawAwait = false
		v.sawLinkCB = false
		v.establishCount = 0
		v.newLinkCount = 0
		v.pathWaitPos = 0
	case *ast.ForStmt, *ast.RangeStmt:
		v.withLoop(node, func() {
			ast.Inspect(node, func(inner ast.Node) bool {
				if inner == node {
					return true
				}
				if call, ok := inner.(*ast.CallExpr); ok {
					v.checkLoopCall(call)
				}
				return true
			})
		})
		return false
	case *ast.SelectStmt:
		v.withLoop(node, func() {
			ast.Inspect(node, func(inner ast.Node) bool {
				if inner == node {
					return true
				}
				if call, ok := inner.(*ast.CallExpr); ok {
					v.checkLoopCall(call)
				}
				return true
			})
		})
		return false
	case *ast.ExprStmt:
		if call, ok := node.X.(*ast.CallExpr); ok {
			v.checkDiscardedPathCall(call)
		}
	case *ast.CallExpr:
		v.checkCall(node)
	}
	return true
}

func (v *visitor) withLoop(node ast.Node, fn func()) {
	v.loopDepth++
	fn()
	v.loopDepth--
	_ = node
}

func (v *visitor) checkLoopCall(call *ast.CallExpr) {
	base := baseName(callName(call))
	pos := v.fset.Position(call.Pos())
	switch {
	case isPathRequestBase(base):
		v.report(RuleRequestPathLoop, pos.Line, pos.Column, nil)
	case base == "AwaitPath":
		v.report(RuleAwaitInLoop, pos.Line, pos.Column, nil)
	case isPathAwaitBase(base) && base == "HasPath":
		v.report(RuleHasPathLoop, pos.Line, pos.Column, nil)
	case isLinkEstablishBase(base):
		v.report(RuleEstablishLoop, pos.Line, pos.Column, nil)
	case isLinkCreateBase(base):
		v.report(RuleNewLinkLoop, pos.Line, pos.Column, nil)
	case isLinkActiveUseBase(base):
		v.report(RuleLinkActiveUseLoop, pos.Line, pos.Column, nil)
	case isAnnounceBase(base):
		v.report(RuleAnnounceLoop, pos.Line, pos.Column, nil)
	}
}

func (v *visitor) checkDiscardedPathCall(call *ast.CallExpr) {
	base := baseName(callName(call))
	if !isPathRequestBase(base) {
		return
	}
	if !funcReturnsError(v.fn) {
		return
	}
	pos := v.fset.Position(call.Pos())
	start := v.fset.Position(call.Pos())
	end := v.fset.Position(call.End())
	from := v.fileRange(start.Offset, end.Offset)
	if from == "" {
		return
	}
	v.report(RuleRequestPathIgnoredErr, pos.Line, pos.Column, &Fix{From: []byte(from), To: fmt.Appendf(nil, "if err := %s; err != nil {\n\t\treturn err\n\t}", from)})
}

func (v *visitor) checkCall(call *ast.CallExpr) {
	base := baseName(callName(call))
	pos := v.fset.Position(call.Pos())

	if isPathAwaitBase(base) {
		v.sawAwait = true
		if v.pathWaitPos == 0 {
			v.pathWaitPos = call.Pos()
		}
	}
	if isLinkCallbackBase(base) {
		v.sawLinkCB = true
	}
	if isRecallBase(base) {
		if callBeforePathWait(call.Pos(), v.pathWaitPos) {
			v.report(RuleRecallBeforePath, pos.Line, pos.Column, nil)
		}
	}
	if isPathRequestBase(base) && requestPathOnInterface(call) {
		v.report(RuleOnInterfaceOverride, pos.Line, pos.Column, nil)
	}
	if isLinkEstablishBase(base) {
		v.establishCount++
		if !v.sawAwait {
			v.report(RuleEstablishNoAwait, pos.Line, pos.Column, nil)
		}
		if v.establishCount > 1 {
			v.report(RuleEstablishRepeat, pos.Line, pos.Column, nil)
		}
	}
	if isLinkCreateBase(base) {
		v.newLinkCount++
		if v.newLinkCount > 1 {
			v.report(RuleNewLinkRepeat, pos.Line, pos.Column, nil)
		}
	}
	if isLinkActiveUseBase(base) && !v.sawLinkCB {
		v.report(RuleLinkNotActive, pos.Line, pos.Column, nil)
	}

	if lit := fifteenSecondArg(call); lit != nil {
		lpos := v.fset.Position(lit.Pos())
		v.report(RuleFixed15sTimeout, lpos.Line, lpos.Column, nil)
	}
}

func (v *visitor) report(ruleID string, line, col int, fix *Fix) {
	v.findings = append(v.findings, NewFinding(ruleID, v.file, line, col, fix))
}

func (v *visitor) fileRange(start, end int) string {
	if v.src == nil {
		data, err := os.ReadFile(v.file) // #nosec G304 -- source file under scan root
		if err != nil {
			return ""
		}
		v.src = data
	}
	if start < 0 {
		start = 0
	}
	if end > len(v.src) {
		end = len(v.src)
	}
	if start >= end {
		return ""
	}
	return string(v.src[start:end])
}

func fifteenSecondArg(call *ast.CallExpr) ast.Expr {
	if call == nil {
		return nil
	}
	name := callName(call)
	base := baseName(name)
	if base != "Sleep" && base != "After" {
		return nil
	}
	if len(call.Args) != 1 {
		return nil
	}
	return matchFifteenSeconds(call.Args[0])
}

func matchFifteenSeconds(expr ast.Expr) ast.Expr {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		if isFifteen(e.X) && isTimeSecond(e.Y) {
			return expr
		}
		if isFifteen(e.Y) && isTimeSecond(e.X) {
			return expr
		}
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Duration" {
			if len(e.Args) == 1 && isFifteen(e.Args[0]) {
				return expr
			}
		}
	}
	return nil
}

func isFifteen(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	return ok && lit.Kind == token.INT && lit.Value == "15"
}

func isTimeSecond(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Second"
}
