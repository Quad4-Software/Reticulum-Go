// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import "go/ast"

func callName(call *ast.CallExpr) string {
	if call == nil || call.Fun == nil {
		return ""
	}
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		if id, ok := f.X.(*ast.Ident); ok {
			return id.Name + "." + f.Sel.Name
		}
		return f.Sel.Name
	default:
		return ""
	}
}

func baseName(name string) string {
	if i := lastDot(name); i >= 0 {
		return name[i+1:]
	}
	return name
}

func lastDot(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

func isPathRequestBase(base string) bool {
	switch base {
	case "RequestPath", "NudgePathRequest", "PrepareFreshPathRequest":
		return true
	default:
		return false
	}
}

func isPathAwaitBase(base string) bool {
	switch base {
	case "AwaitPath", "HasPath":
		return true
	default:
		return false
	}
}

func isLinkEstablishBase(base string) bool {
	return base == "Establish" || base == "TryBeginOutboundEstablish"
}

func isLinkCreateBase(base string) bool {
	return base == "NewLink"
}

func isLinkActiveUseBase(base string) bool {
	switch base {
	case "Send", "Request", "Identify", "SendPacket", "SendResource":
		return true
	default:
		return false
	}
}

func isAnnounceBase(base string) bool {
	return base == "Announce"
}

func isLinkCallbackBase(base string) bool {
	switch base {
	case "SetEstablishedCallback", "SetLinkEstablishedCallback",
		"SetLinkClosedCallback", "SetClosedCallback":
		return true
	default:
		return false
	}
}

func funcReturnsError(fn *ast.FuncDecl) bool {
	if fn == nil || fn.Type == nil || fn.Type.Results == nil {
		return false
	}
	for _, f := range fn.Type.Results.List {
		if id, ok := f.Type.(*ast.Ident); ok && id.Name == "error" {
			return true
		}
	}
	return false
}
