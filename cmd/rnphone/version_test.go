// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRnphoneVersion(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "rnphone")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build rnphone: %v\n%s", err, out)
	}
	got, err := exec.Command(bin, "-version").CombinedOutput()
	if err != nil {
		t.Fatalf("rnphone -version: %v\n%s", err, got)
	}
	if !strings.Contains(string(got), "rnphone 0.5.4") {
		t.Fatalf("version %q", got)
	}
}
