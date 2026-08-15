// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package term

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestNO_COLORDisables(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	if ColorEnabled(os.Stdout) {
		t.Fatal("NO_COLOR should disable color")
	}
}

func TestFORCE_COLOREnables(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "1")
	if !ColorEnabled(os.Stdout) {
		t.Fatal("FORCE_COLOR should enable color")
	}
}

func TestProgressClearNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if ProgressClear(os.Stderr) != "\r" {
		t.Fatal("expected plain CR without color")
	}
}

func TestWrappersHonorFORCE_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "1")
	if !strings.Contains(Green(os.Stdout, "ok"), "\033[32m") {
		t.Fatal("Green should wrap with FORCE_COLOR")
	}
	if !strings.Contains(Red(os.Stdout, "err"), "\033[31m") {
		t.Fatal("Red should wrap with FORCE_COLOR")
	}
	if !strings.Contains(Yellow(os.Stdout, "warn"), "\033[33m") {
		t.Fatal("Yellow should wrap with FORCE_COLOR")
	}
	if !strings.Contains(Cyan(os.Stdout, "info"), "\033[36m") {
		t.Fatal("Cyan should wrap with FORCE_COLOR")
	}
	if !strings.Contains(Bold(os.Stdout, "b"), "\033[1m") {
		t.Fatal("Bold should wrap with FORCE_COLOR")
	}
}

func TestWriterHelpersSkipNonFile(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	var buf bytes.Buffer
	if GreenW(&buf, "x") != "x" {
		t.Fatal("non-file writer should not color")
	}
	if FileOf(&buf) != nil {
		t.Fatal("FileOf should be nil for buffer")
	}
	if FileOf(os.Stdout) != os.Stdout {
		t.Fatal("FileOf should return stdout")
	}
}
