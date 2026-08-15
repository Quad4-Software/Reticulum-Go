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

type pyScope struct {
	pathWaitLine    int
	requestPathLine int
	inWhile         bool
	inLinkWait      bool
}

func scanPython(path, src string) []Finding {
	var out []Finding
	lines := strings.Split(src, "\n")
	scope := pyScope{}

	for i, line := range lines {
		trim := strings.TrimSpace(line)
		lineno := i + 1

		if strings.HasPrefix(trim, "def ") {
			scope = pyScope{}
		}

		if strings.Contains(trim, "require_shared_instance=True") || strings.Contains(trim, "require_shared_instance = True") {
			out = append(out, NewFinding(RulePythonRequireShared, path, lineno, 1, nil))
		}
		if lineHasAny(trim, "on_interface=", "on_interface =") && lineHasAny(trim, "request_path", "await_path") {
			out = append(out, NewFinding(RulePythonOnInterface, path, lineno, 1, nil))
		}

		if strings.HasPrefix(trim, "while ") || strings.HasPrefix(trim, "for ") {
			scope.inWhile = true
		}

		if strings.Contains(trim, "while not") && lineHasAny(trim, "link_ready", "link_failed") {
			scope.inLinkWait = true
		}

		if scope.inLinkWait && strings.Contains(trim, "time.sleep") {
			out = append(out, NewFinding(RulePythonLinkSpin, path, lineno, 1, nil))
			scope.inLinkWait = false
		}

		if scope.inWhile && strings.Contains(trim, "Transport.has_path") {
			out = append(out, NewFinding(RulePythonPathSpin, path, lineno, 1, nil))
			if scope.requestPathLine > 0 && scope.requestPathLine < lineno {
				out = append(out, NewFinding(RulePythonPathThenSpin, path, lineno, 1, nil))
			}
		}
		if scope.inWhile && strings.Contains(trim, "Transport.request_path") {
			out = append(out, NewFinding(RulePythonRequestPathLoop, path, lineno, 1, nil))
		}
		if scope.inWhile && strings.Contains(trim, "await_path") {
			out = append(out, NewFinding(RulePythonAwaitInLoop, path, lineno, 1, nil))
		}
		if scope.inWhile && lineHasAny(trim, "link.status", "Link.ACTIVE", "link.status ==") && strings.Contains(trim, "time.sleep") {
			out = append(out, NewFinding(RulePythonLinkStatusSpin, path, lineno, 1, nil))
		}
		if scope.inWhile && lineHasAny(trim, "link.status", "Link.ACTIVE") {
			// status check in loop body, sleep may be on next line
			if i+1 < len(lines) && strings.Contains(strings.TrimSpace(lines[i+1]), "time.sleep") {
				out = append(out, NewFinding(RulePythonLinkStatusSpin, path, lineno, 1, nil))
			}
		}

		if strings.Contains(trim, "await_path") && !scope.inWhile {
			if scope.pathWaitLine == 0 {
				scope.pathWaitLine = lineno
			}
		}
		if strings.Contains(trim, "Transport.has_path") && !scope.inWhile && !strings.HasPrefix(trim, "while ") {
			if scope.pathWaitLine == 0 {
				scope.pathWaitLine = lineno
			}
		}
		if strings.Contains(trim, ".recall(") || strings.Contains(trim, "Identity.recall(") {
			if scope.pathWaitLine == 0 || lineno < scope.pathWaitLine {
				out = append(out, NewFinding(RulePythonRecallBeforePath, path, lineno, 1, nil))
			}
		}

		if strings.Contains(trim, "Transport.request_path") && !scope.inWhile {
			scope.requestPathLine = lineno
		}

		if strings.Contains(trim, "LINK_TIMEOUT") && strings.Contains(trim, "= 15") {
			out = append(out, NewFinding(RulePythonFixed15s, path, lineno, 1, nil))
		}

		if strings.HasPrefix(trim, "while ") || strings.HasPrefix(trim, "for ") {
			continue
		}
		if trim == "" || strings.HasPrefix(trim, "#") {
			scope.inWhile = false
		}
	}
	return out
}
