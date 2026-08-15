// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"os"
	"path/filepath"
	"strings"
)

func analyzePython(root string) ([]Finding, error) {
	var out []Finding
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".py") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, scanPython(path, string(data))...)
		return nil
	})
	return dedupeFindings(out), err
}

func scanPython(path, src string) []Finding {
	var out []Finding
	lines := strings.Split(src, "\n")
	inLinkWait := false
	inWhile := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		lineno := i + 1

		if strings.HasPrefix(trim, "while ") {
			inWhile = true
		}
		if strings.HasPrefix(trim, "for ") {
			inWhile = true
		}
		if strings.HasPrefix(trim, "def ") {
			inWhile = false
			inLinkWait = false
		}

		if strings.Contains(trim, "while not") && (strings.Contains(trim, "link_ready") || strings.Contains(trim, "link_failed")) {
			inLinkWait = true
		}
		if inLinkWait && strings.Contains(trim, "time.sleep") {
			out = append(out, NewFinding(RulePythonLinkSpin, path, lineno, 1, nil))
			inLinkWait = false
		}
		if inWhile && strings.Contains(trim, "Transport.has_path") {
			out = append(out, NewFinding(RulePythonPathSpin, path, lineno, 1, nil))
		}
		if inWhile && strings.Contains(trim, "Transport.request_path") {
			out = append(out, NewFinding(RulePythonRequestPathLoop, path, lineno, 1, nil))
		}
		if strings.Contains(trim, "LINK_TIMEOUT") && strings.Contains(trim, "= 15") {
			out = append(out, NewFinding(RulePythonFixed15s, path, lineno, 1, nil))
		}
		if strings.HasPrefix(trim, "while ") || strings.HasPrefix(trim, "for ") {
			continue
		}
		if trim == "" || strings.HasPrefix(trim, "#") {
			inWhile = false
		}
	}
	return out
}
