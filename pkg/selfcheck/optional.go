// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package selfcheck

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	py := strings.TrimSpace(os.Getenv("PYTHON_INTEROP"))
	if py == "" {
		for _, cand := range []string{
			filepath.Join(".venv", "bin", "python"),
			filepath.Join(".venv", "bin", "python3"),
			filepath.Join(".venv", "Scripts", "python.exe"),
		} {
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				py = cand
				break
			}
		}
	}
	if py == "" {
		py = "python3"
	}
	py, ok := sanitizePythonInterp(py)
	if !ok {
		return result("interop/python-rns", SeveritySkip, "unsafe PYTHON_INTEROP path")
	}
	if _, err := exec.LookPath(py); err != nil {
		if filepath.IsAbs(py) || strings.Contains(py, string(os.PathSeparator)) {
			if _, err := os.Stat(py); err != nil { // #nosec G703 -- path cleaned by sanitizePythonInterp
				return result("interop/python-rns", SeveritySkip, "python not found")
			}
		} else {
			return result("interop/python-rns", SeveritySkip, py+" not on PATH")
		}
	}
	want := os.Getenv("RNS_REQUIRED_VERSION")
	if want == "" {
		want = "1.3.9"
	}
	cmd := exec.Command(py, "-c", "import RNS; print(getattr(RNS, '__version__', ''))") // #nosec G204,G702 -- python from PATH, .venv, or PYTHON_INTEROP
	out, err := cmd.CombinedOutput()
	if err != nil {
		return result("interop/python-rns", SeveritySkip, "RNS not importable")
	}
	ver := string(bytes.TrimSpace(out))
	if ver == "" {
		return result("interop/python-rns", SeveritySkip, "RNS version unknown")
	}
	if ver != want {
		return result("interop/python-rns", SeverityFail, "got "+ver+" want "+want+" via "+py)
	}
	return result("interop/python-rns", SeverityPass, ver+" via "+py)
}

// sanitizePythonInterp accepts bare interpreter names or cleaned paths without "..".
func sanitizePythonInterp(py string) (string, bool) {
	py = strings.TrimSpace(py)
	if py == "" {
		return "", false
	}
	if !filepath.IsAbs(py) && !strings.Contains(py, string(os.PathSeparator)) {
		return py, true
	}
	clean := filepath.Clean(py)
	if clean == "." || strings.Contains(clean, "..") {
		return "", false
	}
	return clean, true
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
	var detail strings.Builder
	detail.WriteString(found[0])
	for i := 1; i < len(found); i++ {
		detail.WriteString(", " + found[i])
	}
	return result("interop/bindings", SeverityPass, detail.String())
}
