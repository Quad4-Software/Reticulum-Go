// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package term

import (
	"os"
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
