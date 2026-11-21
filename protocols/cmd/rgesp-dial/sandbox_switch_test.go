// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCLIHasNoSandboxSwitch(t *testing.T) {
	disable := regexp.MustCompile(`(?i)(RGESP_NOSANDBOX|NOSANDBOX|no-sandbox|disable-sandbox|flag\.(Bool|String)\("sandbox)`)
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if disable.Match(src) {
			t.Errorf("%s contains a sandbox disable switch", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
