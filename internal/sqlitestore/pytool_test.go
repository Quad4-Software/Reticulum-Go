// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !js && !wasip1 && !tinygo && ((darwin && (amd64 || arm64)) || (freebsd && (386 || amd64 || arm || arm64)) || (illumos && amd64) || (linux && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || s390x)) || (netbsd && amd64) || (openbsd && (amd64 || arm64)) || windows)

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
