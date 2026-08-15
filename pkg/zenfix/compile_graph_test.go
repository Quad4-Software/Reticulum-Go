// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package zenfix

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDaemonImportGraphOmitsXTools(t *testing.T) {
	modRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "list", "-mod=vendor", "-deps", "./cmd/reticulum-go", "./pkg/zenfix")
	cmd.Dir = modRoot
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOFLAGS=-mod=vendor")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	if bytes.Contains(out, []byte("golang.org/x/tools")) {
		t.Fatal("cmd/reticulum-go or pkg/zenfix still depends on golang.org/x/tools")
	}
}
