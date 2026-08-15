// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"os"
	"path/filepath"
	"testing"

	"go/parser"
	"go/token"
)

// functionOracle maps corpus function names to rules that must fire at least once.
var badOracle = map[string][]string{
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

var linkOracle = map[string][]string{
	"BadNewLinkLoop":        {"zen/newlink-loop"},
	"BadLinkRequestLoop":    {"zen/link-active-use-loop"},
	"BadLinkSendNoCallback": {"zen/link-not-active"},
	"BadNewLinkRepeat":      {"zen/newlink-repeat"},
	"BadLinkSendLoop":       {"zen/link-active-use-loop"},
}

var goodOracle = map[string][]string{
	"GoodPath": {},
	"GoodLink": {},
}

var adversarialOracle = map[string][]string{
	"AdversarialNested":       {"zen/requestpath-loop"},
	"AdversarialCleanLiteral": {},
}

func TestOracleBadPackage(t *testing.T) {
	assertFuncOracle(t, filepath.Join("testdata", "module", "bad", "bad.go"), badOracle)
}

func TestOracleBadRecallFile(t *testing.T) {
	assertFuncOracle(t, filepath.Join("testdata", "module", "bad", "recall.go"), map[string][]string{
		"BadRecallBeforePath": {RuleRecallBeforePath},
		"BadOnInterface":      {RuleOnInterfaceOverride},
	})
}

func TestOracleLinkPackage(t *testing.T) {
	assertFuncOracle(t, filepath.Join("testdata", "module", "linkbad", "linkbad.go"), linkOracle)
}

func TestOracleGoodPackage(t *testing.T) {
	assertFuncOracle(t, filepath.Join("testdata", "module", "good", "good.go"), goodOracle)
}

func TestOracleAdversarialPackage(t *testing.T) {
	assertFuncOracle(t, filepath.Join("testdata", "module", "adversarial", "adversarial.go"), adversarialOracle)
}

func TestOracleRNSPythonPatterns(t *testing.T) {
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

func assertFuncOracle(t *testing.T, path string, oracle map[string][]string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	findings := inspectFile(fset, path, file)
	byFunc := rulesByFunc(fset, file, findings)
	for funcName, wantRules := range oracle {
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
