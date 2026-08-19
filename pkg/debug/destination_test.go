// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package debug

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestConfigureDestinationFile(t *testing.T) {
	dir := t.TempDir()
	cfg := &common.ReticulumConfig{
		ConfigPath:     filepath.Join(dir, "config"),
		LogDestination: "file",
		LogLevel:       3,
	}
	Init()
	SetDebugLevel(DebugInfo)
	if err := ConfigureDestination(cfg); err != nil {
		t.Fatal(err)
	}
	Log(DebugInfo, "hello file log")
	path := filepath.Join(dir, "logfile", "reticulum.log")
	b, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "hello file log") {
		t.Fatalf("logfile missing message: %q", b)
	}
}

func TestSetJSONFormat(t *testing.T) {
	Init()
	SetJSONFormat(true)
	SetDebugLevel(DebugInfo)
	Log(DebugInfo, "json line")
	SetJSONFormat(false)
}
