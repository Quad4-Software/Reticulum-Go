// SPDX-License-Identifier: LicenseRef-Reticulum
package main

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestRunVersion(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := run([]string{"--version"})
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !bytes.Contains(out, []byte("0.1.0")) && len(bytes.TrimSpace(out)) == 0 {
		t.Fatalf("version output %q", out)
	}
}

func TestRunUnknownFlag(t *testing.T) {
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	old := os.Stderr
	os.Stderr = devnull
	code := run([]string{"--not-a-flag"})
	os.Stderr = old
	if code != 2 {
		t.Fatalf("exit=%d", code)
	}
}
