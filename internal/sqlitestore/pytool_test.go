// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !js && !wasip1 && !tinygo

package sqlitestore

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func runPython(t *testing.T, python, src string, args ...string) (string, error) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "probe.py")
	if err := os.WriteFile(f, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, append([]string{f}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
