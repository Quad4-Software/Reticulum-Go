// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

// Severity ranks how likely a pattern is to harm the mesh or mislead the app.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityHint    Severity = "hint"
)

// Finding is one zen / footgun report.
type Finding struct {
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Col      int      `json:"col,omitempty"`
	EndLine  int      `json:"end_line,omitempty"`
	Message  string   `json:"message"`
	Why      string   `json:"why,omitempty"`
	Hint     string   `json:"hint,omitempty"`
	Refs     []string `json:"refs,omitempty"`
	Fix      *Fix     `json:"-"`
}

// Fix is an optional rewrite for -fix mode.
type Fix struct {
	From []byte
	To   []byte
}

// Summary counts findings by severity.
type Summary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Hints    int `json:"hints"`
	Fixed    int `json:"fixed,omitempty"`
}

func summarize(findings []Finding) Summary {
	var s Summary
	for _, f := range findings {
		switch f.Severity {
		case SeverityError:
			s.Errors++
		case SeverityWarning:
			s.Warnings++
		default:
			s.Hints++
		}
	}
	return s
}
