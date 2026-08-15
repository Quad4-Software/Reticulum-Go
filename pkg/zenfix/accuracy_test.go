// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/transport"
)

func TestAccuracyPathRequestMI(t *testing.T) {
	if transport.PathRequestMI != 20*time.Second {
		t.Fatalf("PathRequestMI=%s, update zen rules if this changed", transport.PathRequestMI)
	}
	r, ok := LookupRule(RuleRequestPathIgnoredErr)
	if !ok {
		t.Fatal("missing rule")
	}
	if !strings.Contains(r.Why, "20 second") {
		t.Fatalf("rule why should mention 20 second window: %q", r.Why)
	}
}

func TestAccuracyAnnounceThrottle(t *testing.T) {
	// Values from pkg/destination/constants.go announceBurstMax and announceBurstWindow.
	r, ok := LookupRule(RuleAnnounceLoop)
	if !ok {
		t.Fatal("missing rule")
	}
	if !strings.Contains(r.Why, "8") || !strings.Contains(r.Why, "10 second") {
		t.Fatalf("announce rule why out of sync with destination/constants.go: %q", r.Why)
	}
}

func TestRuleDocRefsExist(t *testing.T) {
	root := repoRoot(t)
	for _, rule := range AllRules {
		for _, ref := range rule.Refs {
			path := filepath.Join(root, ref)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("rule %s ref %s: %v", rule.ID, ref, err)
			}
		}
	}
}

func TestCatalogIDsBidirectional(t *testing.T) {
	want := []string{
		RuleRequestPathLoop,
		RuleRequestPathIgnoredErr,
		RuleHasPathLoop,
		RuleAwaitInLoop,
		RuleEstablishLoop,
		RuleEstablishNoAwait,
		RuleEstablishRepeat,
		RuleNewLinkLoop,
		RuleNewLinkRepeat,
		RuleLinkNotActive,
		RuleLinkActiveUseLoop,
		RuleAnnounceLoop,
		RuleFixed15sTimeout,
		RulePythonLinkSpin,
		RulePythonPathSpin,
		RulePythonRequestPathLoop,
		RulePythonFixed15s,
	}
	if len(AllRules) != len(want) {
		t.Fatalf("catalog len %d want %d", len(AllRules), len(want))
	}
	seen := make(map[string]struct{}, len(want))
	for _, id := range want {
		seen[id] = struct{}{}
	}
	for _, r := range AllRules {
		if _, ok := seen[r.ID]; !ok {
			t.Fatalf("unexpected catalog id %s", r.ID)
		}
		if r.ID != ruleByID[r.ID].ID {
			t.Fatalf("index mismatch for %s", r.ID)
		}
	}
}

func TestCorpusFindingsMatchCatalog(t *testing.T) {
	dir := filepath.Join("testdata", "module")
	res, err := Run(Options{Dir: dir, Patterns: []string{"./..."}, Python: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) == 0 {
		t.Fatal("expected corpus findings")
	}
	for _, f := range res.Findings {
		assertFindingMatchesCatalog(t, f)
	}
}

func assertFindingMatchesCatalog(t *testing.T, f Finding) {
	t.Helper()
	r, ok := LookupRule(f.Rule)
	if !ok {
		t.Fatalf("unknown rule %s at %s:%d", f.Rule, f.File, f.Line)
	}
	if f.Message != r.Title {
		t.Fatalf("%s message drift: got %q want %q", f.Rule, f.Message, r.Title)
	}
	if f.Why != r.Why {
		t.Fatalf("%s why drift at %s:%d", f.Rule, f.File, f.Line)
	}
	if f.Hint != r.Hint {
		t.Fatalf("%s hint drift at %s:%d", f.Rule, f.File, f.Line)
	}
	if f.Severity != r.Severity {
		t.Fatalf("%s severity drift: got %s want %s", f.Rule, f.Severity, r.Severity)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "docs", "en")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}
