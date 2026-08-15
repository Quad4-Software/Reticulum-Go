// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package selfcheck

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Severity classifies a single check outcome.
type Severity string

const (
	SeverityPass Severity = "pass"
	SeverityWarn Severity = "warn"
	SeverityFail Severity = "fail"
	SeveritySkip Severity = "skip"
)

// Result is one named check outcome.
type Result struct {
	Name     string   `json:"name"`
	Severity Severity `json:"severity"`
	Detail   string   `json:"detail,omitempty"`
	Platform string   `json:"platform,omitempty"`
}

// Report aggregates host self-check results.
type Report struct {
	GOOS      string   `json:"goos"`
	GOARCH    string   `json:"goarch"`
	GoVersion string   `json:"go_version"`
	Results   []Result `json:"results"`
}

// Counts returns pass, warn, skip, and fail totals.
func (r Report) Counts() (pass, warn, skip, fail int) {
	for _, res := range r.Results {
		switch res.Severity {
		case SeverityPass:
			pass++
		case SeverityWarn:
			warn++
		case SeveritySkip:
			skip++
		case SeverityFail:
			fail++
		}
	}
	return pass, warn, skip, fail
}

// ExitCode returns 0 when there are no failures.
// When strict is true, warnings also yield exit code 1.
func (r Report) ExitCode(strict bool) int {
	_, warn, _, fail := r.Counts()
	if fail > 0 {
		return 1
	}
	if strict && warn > 0 {
		return 1
	}
	return 0
}

// FormatText writes a human-readable report to w.
func (r Report) FormatText(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "reticulum-go self-check  %s/%s  %s\n", r.GOOS, r.GOARCH, r.GoVersion); err != nil {
		return err
	}
	for _, res := range r.Results {
		line := fmt.Sprintf("  [%s] %s", res.Severity, res.Name)
		if res.Detail != "" {
			line += ": " + res.Detail
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	pass, warn, skip, fail := r.Counts()
	_, err := fmt.Fprintf(w, "Summary: %d pass, %d warn, %d skip, %d fail\n", pass, warn, skip, fail)
	return err
}

// FormatJSON writes an indented JSON report to w.
func (r Report) FormatJSON(w io.Writer) error {
	pass, warn, skip, fail := r.Counts()
	out := struct {
		Report
		Pass int `json:"pass"`
		Warn int `json:"warn"`
		Skip int `json:"skip"`
		Fail int `json:"fail"`
	}{
		Report: r,
		Pass:   pass,
		Warn:   warn,
		Skip:   skip,
		Fail:   fail,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func platformTag() string {
	return fmt.Sprintf("%s/%s", goos(), goarch())
}

func result(name string, sev Severity, detail string) Result {
	return Result{
		Name:     name,
		Severity: sev,
		Detail:   strings.TrimSpace(detail),
		Platform: platformTag(),
	}
}
