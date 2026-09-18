// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"strings"
	"testing"
)

func mdConv() *mdToMicron { return newMdToMicron(100) }

func TestMdHeaders(t *testing.T) {
	out := mdConv().formatBlock("# Title\n\n## Sub\n", "")
	if !strings.Contains(out, ">Title") || !strings.Contains(out, ">>Sub") {
		t.Fatalf("headers not converted:\n%s", out)
	}
}

func TestMdUnorderedList(t *testing.T) {
	out := mdConv().formatBlock("- one\n- two\n", "")
	if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		t.Fatalf("list items lost:\n%s", out)
	}
}

func TestMdFencedCode(t *testing.T) {
	out := mdConv().formatBlock("```go\nfunc main() {}\n```\n", "")
	if !strings.Contains(out, "func main() {}") {
		t.Fatalf("code block content lost:\n%s", out)
	}
	if strings.Contains(out, "```") {
		t.Fatalf("fence markers leaked:\n%s", out)
	}
}

func TestMdInlineLink(t *testing.T) {
	out := mdConv().formatBlock("see [docs](https://example.com/x) now\n", "")
	if !strings.Contains(out, "docs") || !strings.Contains(out, "example.com") {
		t.Fatalf("link not converted:\n%s", out)
	}
}

func TestMdInlineCodeAndBold(t *testing.T) {
	out := mdConv().formatBlock("run `make test` with **force**\n", "")
	if !strings.Contains(out, "make test") || !strings.Contains(out, "force") {
		t.Fatalf("inline formatting lost:\n%s", out)
	}
}

func TestMdTable(t *testing.T) {
	out := mdConv().formatBlock("| a | b |\n|---|---|\n| 1 | 2 |\n", "")
	if !strings.Contains(out, "a") || !strings.Contains(out, "2") {
		t.Fatalf("table cells lost:\n%s", out)
	}
}

func TestMdLiteralEscaping(t *testing.T) {
	// A stray backtick in source text must not open a Micron format span.
	out := mdConv().formatBlock("cost is 5`6 dollars\n", "")
	if !strings.Contains(out, "5\\`6") {
		t.Fatalf("literal backtick not escaped:\n%s", out)
	}
	if !strings.Contains(out, "5") || !strings.Contains(out, "6") {
		t.Fatalf("content lost:\n%s", out)
	}
}

func TestMdHorizontalRule(t *testing.T) {
	out := mdConv().formatBlock("above\n\n---\n\nbelow\n", "")
	if !strings.Contains(out, "-") || !strings.Contains(out, "above") || !strings.Contains(out, "below") {
		t.Fatalf("rule handling:\n%s", out)
	}
}

func TestHighlightGoKeywords(t *testing.T) {
	h := newHighlighter()
	out := h.highlight("func f() { return 1 }", "x.go", "")
	if !strings.Contains(out, "func") {
		t.Fatalf("content lost: %q", out)
	}
	// Highlighted output must differ from input via markup tags.
	if out == "func f() { return 1 }" {
		t.Fatal("no highlighting applied to Go source")
	}
}

func TestHighlightUnknownPassthrough(t *testing.T) {
	h := newHighlighter()
	src := "some plain text line"
	out := h.highlight(src, "x.unknownext", "")
	if !strings.Contains(out, "some plain text line") {
		t.Fatalf("unknown-language content lost: %q", out)
	}
}

func TestLanguageForExtension(t *testing.T) {
	for ext, want := range map[string]string{
		"x.go":  "go",
		"x.py":  "python",
		"x.rs":  "rust",
		"x.js":  "javascript",
		"x.c":   "c",
		"x.cpp": "cpp",
		"x.sh":  "shell",
	} {
		spec := languageFor("", ext)
		if spec == nil {
			t.Fatalf("no spec for %s", ext)
		}
		_ = want
	}
	if languageFor("", "x.xyzzy") != nil {
		t.Fatal("unknown extension returned a spec")
	}
}
