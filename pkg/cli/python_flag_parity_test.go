// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPathHelpMatchesPythonShortFlags(t *testing.T) {
	var stderr bytes.Buffer
	code := RunPath([]string{"-h"}, Options{Stderr: &stderr})
	if code != 2 && code != 0 {
		// flag.ContinueOnError returns Parse error as 2 from RunPath
	}
	out := stderr.String()
	for _, want := range []string{
		"-D", "drop all queued announces",
		"-x", "drop all paths via",
		"-b", "list blackholed",
		"-B", "blackhole identity",
		"-U", "lift blackhole",
		"-p", "published blackhole",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("rgopath -h missing %q\n%s", want, out)
		}
	}
}

func TestCPHelpMatchesPythonShortFlags(t *testing.T) {
	var stderr bytes.Buffer
	_ = RunCP([]string{"-h"}, Options{Stderr: &stderr})
	out := stderr.String()
	for _, want := range []string{
		"-a", "allowed identity",
		"-n", "accept requests from anyone",
		"-F", "allow authenticated clients to fetch",
		"-S", "silent",
		"-C", "disable auto compression",
		"-P", "physical layer",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("rgocp -h missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "-F string") {
		t.Fatalf("rgocp -F must be allow-fetch bool, not fetch path string\n%s", out)
	}
}

func TestStatusHelpMatchesPythonShortFlags(t *testing.T) {
	var stderr bytes.Buffer
	_ = RunStatus([]string{"-h"}, Options{Stderr: &stderr})
	out := stderr.String()
	for _, want := range []string{
		"-p", "packets per second",
		"-m", "continuously monitor",
		"-I", "refresh interval",
		"-z", "profiling",
		"-Q", "queue pressure",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("rgostatus -h missing %q\n%s", want, out)
		}
	}
}
