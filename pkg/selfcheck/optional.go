// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package selfcheck

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
)

func checkCrossref() Result {
	candidates := []string{
		"tests/crossref/test_vectors.json",
		filepath.Join("..", "..", "tests", "crossref", "test_vectors.json"),
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "tests", "crossref", "test_vectors.json"),
			filepath.Join(wd, "..", "..", "tests", "crossref", "test_vectors.json"),
		)
	}
	for _, p := range candidates {
		st, err := os.Stat(p)
		if err == nil && st.Size() > 0 {
			return result("interop/crossref", SeverityPass, p)
		}
	}
	return result("interop/crossref", SeveritySkip, "test_vectors.json not found")
}

func checkPythonRNS() Result {
	py := os.Getenv("PYTHON_INTEROP")
	if py == "" {
		py = "python3"
	}
	if _, err := exec.LookPath(py); err != nil {
		return result("interop/python-rns", SeveritySkip, py+" not on PATH")
	}
	cmd := exec.Command(py, "-c", "import RNS; print(getattr(RNS, '__version__', 'ok'))") // #nosec G204,G702 -- python from PATH or PYTHON_INTEROP
	out, err := cmd.CombinedOutput()
	if err != nil {
		return result("interop/python-rns", SeveritySkip, "RNS not importable")
	}
	return result("interop/python-rns", SeverityPass, string(bytes.TrimSpace(out)))
}

func checkBindings() Result {
	found := make([]string, 0, 3)
	if _, err := exec.LookPath("odin"); err == nil {
		found = append(found, "odin")
	}
	if _, err := exec.LookPath("dart"); err == nil {
		found = append(found, "dart")
	}
	for _, p := range []string{"bin/librns.so", "librns.so"} {
		if _, err := os.Stat(p); err == nil {
			found = append(found, p)
			break
		}
	}
	if len(found) == 0 {
		return result("interop/bindings", SeveritySkip, "no binding tools detected")
	}
	detail := found[0]
	for i := 1; i < len(found); i++ {
		detail += ", " + found[i]
	}
	return result("interop/bindings", SeverityPass, detail)
}
