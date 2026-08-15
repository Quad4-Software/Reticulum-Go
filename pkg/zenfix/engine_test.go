// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuleCatalogComplete(t *testing.T) {
	for _, r := range AllRules {
		if r.Title == "" || r.Why == "" || r.Hint == "" {
			t.Fatalf("rule %s missing title/why/hint", r.ID)
		}
		if len(r.Refs) == 0 {
			t.Fatalf("rule %s missing refs", r.ID)
		}
		if _, ok := LookupRule(r.ID); !ok {
			t.Fatalf("index missing %s", r.ID)
		}
	}
}

func TestNewFindingFromCatalog(t *testing.T) {
	f := NewFinding(RuleRequestPathLoop, "a.go", 10, 3, nil)
	if f.Message == "" || f.Why == "" || f.Hint == "" || len(f.Refs) == 0 {
		t.Fatalf("finding not enriched: %+v", f)
	}
	if f.Severity != SeverityError {
		t.Fatalf("severity %s", f.Severity)
	}
}

func TestResultJSONIncludesWhyAndRefs(t *testing.T) {
	f := NewFinding(RuleHasPathLoop, "b.go", 1, 1, nil)
	res := Result{Findings: []Finding{f}, Summary: summarize([]Finding{f})}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, key := range []string{`"why"`, `"refs"`, `"hint"`, RuleHasPathLoop} {
		if !strings.Contains(s, key) {
			t.Fatalf("json missing %s: %s", key, s)
		}
	}
}
