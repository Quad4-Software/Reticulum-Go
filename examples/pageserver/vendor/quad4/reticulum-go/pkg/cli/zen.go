// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"quad4/reticulum-go/pkg/zenfix"
)

// RunZen scans Go (and optionally Python) sources for Reticulum footguns.
func RunZen(args []string, opt ...Options) int {
	stdout, stderr := cliIO(opt)
	fs := flag.NewFlagSet("zen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fix := fs.Bool("fix", false, "apply safe automatic fixes")
	jsonOut := fs.Bool("json", false, "emit JSON report")
	listRules := fs.Bool("list-rules", false, "list all zen rules and exit")
	plain := fs.Bool("plain", false, "plain output without colors or extra detail")
	tests := fs.Bool("test", false, "include *_test.go files")
	python := fs.Bool("python", false, "also scan .py files under the module root")
	dir := fs.String("C", "", "module root directory (default: cwd)")
	bindFlagUsage(fs, "reticulum-go zen - Reticulum footgun scanner",
		"Scans Go (and optionally Python) sources for path/link pitfalls.",
		[]helpLine{
			{Cmd: "reticulum-go zen [flags] [packages]"},
			{Cmd: "rgozen [flags] [packages]"},
		},
		"reticulum-go zen ./...",
		"reticulum-go zen -fix ./pkg/transport",
		"reticulum-go zen -list-rules",
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *listRules {
		printZenRules(stdout, *plain)
		return 0
	}

	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	res, err := zenfix.Run(zenfix.Options{
		Patterns: patterns,
		Dir:      *dir,
		Fix:      *fix,
		Tests:    *tests,
		Python:   *python,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s\n", errMsg(stderr, err.Error()))
		return 1
	}

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			fmt.Fprintf(stderr, "%s\n", errMsg(stderr, err.Error()))
			return 1
		}
	} else {
		for i, f := range res.Findings {
			if i > 0 {
				fmt.Fprintln(stdout)
			}
			printZenFinding(stdout, f, *plain)
		}
		if len(res.Findings) > 0 {
			fmt.Fprintln(stdout)
			printZenSummary(stdout, res.Summary, *plain)
		} else {
			fmt.Fprintln(stdout, okMsg(stdout, "no zen issues found"))
		}
	}

	if res.Summary.Errors > 0 {
		return 1
	}
	if res.Summary.Warnings > 0 && !*fix {
		return 1
	}
	return 0
}

func printZenRules(w io.Writer, plain bool) {
	for i, rule := range zenfix.AllRules {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "%s\n", zenSeverityLabel(w, rule.Severity, plain))
		fmt.Fprintf(w, "  %s\n", zenRuleID(w, rule.ID, plain))
		fmt.Fprintf(w, "  %s\n", rule.Title)
		if rule.Why != "" {
			fmt.Fprintf(w, "  why: %s\n", rule.Why)
		}
		if rule.Hint != "" {
			fmt.Fprintf(w, "  fix: %s\n", rule.Hint)
		}
		for _, ref := range rule.Refs {
			fmt.Fprintf(w, "  see: %s\n", zenRef(w, ref, plain))
		}
		if rule.Fixable {
			fmt.Fprintf(w, "  %s\n", infoMsg(w, "auto-fix available with -fix"))
		}
	}
}

func printZenFinding(w io.Writer, f zenfix.Finding, plain bool) {
	fmt.Fprintf(w, "%s  %s\n", zenSeverityLabel(w, f.Severity, plain), zenRuleID(w, f.Rule, plain))
	fmt.Fprintf(w, "  %s:%d:%d\n", f.File, f.Line, f.Col)
	fmt.Fprintf(w, "\n  %s\n", f.Message)
	if f.Why != "" {
		fmt.Fprintf(w, "\n  %s %s\n", zenLabel(w, "why:", plain), f.Why)
	}
	if f.Hint != "" {
		fmt.Fprintf(w, "\n  %s %s\n", zenLabel(w, "fix:", plain), f.Hint)
	}
	for _, ref := range f.Refs {
		fmt.Fprintf(w, "  %s %s\n", zenLabel(w, "see:", plain), zenRef(w, ref, plain))
	}
}

func printZenSummary(w io.Writer, s zenfix.Summary, plain bool) {
	line := fmt.Sprintf("%d error(s), %d warning(s), %d hint(s)", s.Errors, s.Warnings, s.Hints)
	if s.Fixed > 0 {
		line += fmt.Sprintf(", %d fixed", s.Fixed)
	}
	if plain {
		fmt.Fprintln(w, line)
		return
	}
	switch {
	case s.Errors > 0:
		fmt.Fprintln(w, errMsg(w, line))
	case s.Warnings > 0:
		fmt.Fprintln(w, warnMsg(w, line))
	default:
		fmt.Fprintln(w, infoMsg(w, line))
	}
}

func zenSeverityLabel(w io.Writer, sev zenfix.Severity, plain bool) string {
	label := strings.ToUpper(string(sev))
	if plain {
		return label
	}
	switch sev {
	case zenfix.SeverityError:
		return errMsg(w, label)
	case zenfix.SeverityWarning:
		return warnMsg(w, label)
	default:
		return infoMsg(w, label)
	}
}

func zenRuleID(w io.Writer, id string, plain bool) string {
	if plain {
		return id
	}
	return infoMsg(w, id)
}

func zenLabel(w io.Writer, label string, plain bool) string {
	if plain {
		return label
	}
	return warnMsg(w, label)
}

func zenRef(w io.Writer, ref string, plain bool) string {
	if plain {
		return ref
	}
	return infoMsg(w, ref)
}
