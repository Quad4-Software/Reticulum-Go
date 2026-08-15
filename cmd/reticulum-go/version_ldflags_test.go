// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRegression_ReleaseBuildEmbedsDefaultVersion guards release hygiene:
// tagged assets must pass -X main.defaultVersion so binaries do not report "dev".
func TestRegression_ReleaseBuildEmbedsDefaultVersion(t *testing.T) {
	root := repoRoot(t)
	needle := "-X main.defaultVersion"
	files := []string{
		"Makefile",
		"taskfiles/build.yml",
		".github/workflows/release-assets.yml",
	}
	for _, rel := range files {
		path := filepath.Join(root, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !strings.Contains(string(data), needle) {
			t.Errorf("%s missing %q (release binaries would report dev)", rel, needle)
		}
	}

	buildYML := filepath.Join(root, "taskfiles/build.yml")
	data, err := os.ReadFile(buildYML)
	if err != nil {
		t.Fatalf("read build.yml: %v", err)
	}
	text := string(data)
	// Dynamic Task VERSION: sh: git describe overwrites CLI/env VERSION and
	// breaks release-assets (task build VERSION=v1.0.0).
	if strings.Contains(text, "VERSION:") && strings.Contains(text, "sh: git describe") {
		// Only fail when VERSION itself is the dynamic var, not GIT_DESCRIBE.
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "VERSION:" {
				continue
			}
			if i+1 < len(lines) && strings.Contains(lines[i+1], "sh: git describe") {
				t.Error("taskfiles/build.yml: dynamic sh: VERSION overrides CLI/env; use GIT_DESCRIBE instead")
			}
		}
	}
	if !strings.Contains(text, "GIT_DESCRIBE") {
		t.Error("taskfiles/build.yml: expected GIT_DESCRIBE for default version fallback")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// cmd/reticulum-go -> repo root
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
