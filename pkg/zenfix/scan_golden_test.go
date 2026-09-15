// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"os"
	"path/filepath"
	"testing"

	"go/ast"
	"go/parser"
	"go/token"
)

// goldenRules maps corpus function names to rules that must fire at least once.
var goldenBad = map[string][]string{
	"BadPathLoop":         {"zen/requestpath-loop"},
	"BadNudgeLoop":        {"zen/requestpath-loop"},
	"BadHasPathLoop":      {"zen/haspath-loop"},
	"BadAwaitLoop":        {"zen/await-in-loop"},
	"BadEstablishLoop":    {"zen/establish-loop"},
	"BadIgnoredError":     {"zen/requestpath-ignored-error"},
	"BadEstablishNoAwait": {"zen/establish-no-await"},
	"BadEstablishRepeat":  {"zen/establish-repeat"},
	"BadFixedTimeout":     {"zen/fixed-15s-timeout"},
	"BadAnnounceLoop":     {"zen/announce-loop"},
	"BadSelectLoop":       {"zen/requestpath-loop"},
}

var goldenLink = map[string][]string{
	"BadNewLinkLoop":        {"zen/newlink-loop"},
	"BadLinkRequestLoop":    {"zen/link-active-use-loop"},
	"BadLinkSendNoCallback": {"zen/link-not-active"},
	"BadNewLinkRepeat":      {"zen/newlink-repeat"},
	"BadLinkSendLoop":       {"zen/link-active-use-loop"},
}

var goldenGood = map[string][]string{
	"GoodPath": {},
	"GoodLink": {},
}

var goldenAdversarial = map[string][]string{
	"AdversarialNested":       {"zen/requestpath-loop"},
	"AdversarialCleanLiteral": {},
}

func TestGoldenBadPackage(t *testing.T) {
	assertGoldenRules(t, filepath.Join("testdata", "module", "bad", "bad.go"), goldenBad)
}

func TestGoldenBadRecallFile(t *testing.T) {
	assertGoldenRules(t, filepath.Join("testdata", "module", "bad", "recall.go"), map[string][]string{
		"BadRecallBeforePath": {RuleRecallBeforePath},
		"BadOnInterface":      {RuleOnInterfaceOverride},
	})
}

func TestGoldenLinkPackage(t *testing.T) {
	assertGoldenRules(t, filepath.Join("testdata", "module", "linkbad", "linkbad.go"), goldenLink)
}

func TestGoldenGoodPackage(t *testing.T) {
	assertGoldenRules(t, filepath.Join("testdata", "module", "good", "good.go"), goldenGood)
}

func TestGoldenAdversarialPackage(t *testing.T) {
	assertGoldenRules(t, filepath.Join("testdata", "module", "adversarial", "adversarial.go"), goldenAdversarial)
}

func TestGoldenRNSPythonPatterns(t *testing.T) {
	path := filepath.Join("testdata", "module", "python", "rns_patterns.py")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	findings := scanPython(path, string(b))
	rules := ruleSet(nil)
	for _, f := range findings {
		rules[f.Rule]++
	}
	for _, id := range []string{
		RulePythonPathThenSpin,
		RulePythonAwaitInLoop,
		RulePythonLinkStatusSpin,
		RulePythonRecallBeforePath,
		RulePythonRequireShared,
	} {
		if rules[id] == 0 {
			t.Fatalf("missing %s in rns_patterns.py, got %v", id, rules)
		}
	}
}

func assertGoldenRules(t *testing.T, path string, want map[string][]string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	findings := inspectFile(fset, path, file)
	byFunc := rulesByFunc(fset, file, findings)
	for funcName, wantRules := range want {
		got := byFunc[funcName]
		if len(wantRules) == 0 {
			if len(got) > 0 {
				t.Fatalf("%s: func %s should be clean, got %v", path, funcName, got)
			}
			continue
		}
		gotSet := ruleSet(got)
		for _, rule := range wantRules {
			if gotSet[rule] == 0 {
				t.Fatalf("%s: missing rule %s in func %s (got %v)", path, rule, funcName, got)
			}
		}
	}
}

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
