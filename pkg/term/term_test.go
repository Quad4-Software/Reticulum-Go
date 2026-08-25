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
	t.Setenv("CLICOLOR", "")
	if ColorEnabled(os.Stdout) {
		t.Fatal("NO_COLOR should disable color")
	}
}

func TestCLICOLOR0Disables(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("CLICOLOR", "0")
	if ColorEnabled(os.Stdout) {
		t.Fatal("CLICOLOR=0 should disable color")
	}
}

func TestTERMdumbDisables(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("TERM", "dumb")
	if ColorEnabled(os.Stdout) {
		t.Fatal("TERM=dumb should disable color")
	}
}

func TestFORCE_COLOREnables(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("TERM", "")
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

func TestClearScreenNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if ClearScreen(os.Stdout) != "" {
		t.Fatal("expected empty clear without color")
	}
}

func TestWrappersHonorFORCE_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "1")
	t.Setenv("TERM", "")
	checks := map[string]string{
		Green(os.Stdout, "ok"):    "\033[32m",
		Red(os.Stdout, "err"):     "\033[31m",
		Yellow(os.Stdout, "warn"): "\033[33m",
		Cyan(os.Stdout, "info"):   "\033[36m",
		Blue(os.Stdout, "note"):   "\033[34m",
		Magenta(os.Stdout, "tag"): "\033[35m",
		Bold(os.Stdout, "b"):      "\033[1m",
		Dim(os.Stdout, "d"):       "\033[2m",
		ClearScreen(os.Stdout):    "\033[2J",
	}
	for got, want := range checks {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in %q", want, got)
		}
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
	if ColorEnabledW(&buf) {
		t.Fatal("ColorEnabledW should be false for non-file writer")
	}
}
