// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

// Rule is one catalogued zen check. All user-facing text and doc refs live here.
type Rule struct {
	ID       string   `json:"id"`
	Severity Severity `json:"severity"`
	Title    string   `json:"title"`
	Why      string   `json:"why"`
	Hint     string   `json:"hint"`
	Refs     []string `json:"refs,omitempty"`
	Fixable  bool     `json:"fixable,omitempty"`
}

var ruleByID map[string]Rule

func init() {
	ruleByID = make(map[string]Rule, len(AllRules))
	for _, r := range AllRules {
		ruleByID[r.ID] = r
	}
}

// LookupRule returns catalog metadata for a rule id.
func LookupRule(id string) (Rule, bool) {
	r, ok := ruleByID[id]
	return r, ok
}

// NewFinding builds a finding from the rule catalog plus a source location.
func NewFinding(ruleID, file string, line, col int, fix *Fix) Finding {
	r, ok := ruleByID[ruleID]
	if !ok {
		return Finding{
			Rule:     ruleID,
			Severity: SeverityWarning,
			File:     file,
			Line:     line,
			Col:      col,
			Message:  ruleID,
			Fix:      fix,
		}
	}
	return Finding{
		Rule:     ruleID,
		Severity: r.Severity,
		File:     file,
		Line:     line,
		Col:      col,
		Message:  r.Title,
		Why:      r.Why,
		Hint:     r.Hint,
		Refs:     r.Refs,
		Fix:      fix,
	}
}
