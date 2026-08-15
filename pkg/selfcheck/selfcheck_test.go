// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package selfcheck

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if code, ok := ChildExitCode(); ok {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestReportExitCode(t *testing.T) {
	r := Report{Results: []Result{
		{Severity: SeverityPass},
		{Severity: SeverityWarn},
		{Severity: SeveritySkip},
	}}
	if r.ExitCode(false) != 0 {
		t.Fatalf("expected 0 without strict")
	}
	if r.ExitCode(true) != 1 {
		t.Fatalf("expected 1 with strict warns")
	}
	r.Results = append(r.Results, Result{Severity: SeverityFail})
	if r.ExitCode(false) != 1 {
		t.Fatalf("expected 1 on fail")
	}
}

func TestFormatTextAndJSON(t *testing.T) {
	r := Report{
		GOOS:      "linux",
		GOARCH:    "amd64",
		GoVersion: "go1.26.5",
		Results: []Result{
			result("crypto/ed25519", SeverityPass, "ok"),
			result("sandbox/landlock", SeverityWarn, "unavailable (kernel LSM)"),
		},
	}
	var text bytes.Buffer
	if err := r.FormatText(&text); err != nil {
		t.Fatal(err)
	}
	s := text.String()
	if !strings.Contains(s, "[pass] crypto/ed25519") {
		t.Fatalf("text missing pass: %s", s)
	}
	if !strings.Contains(s, "Summary:") {
		t.Fatalf("text missing summary: %s", s)
	}
	var js bytes.Buffer
	if err := r.FormatJSON(&js); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js.String(), `"severity": "pass"`) {
		t.Fatalf("json: %s", js.String())
	}
}

func TestRunQuick(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	rep := Run(ctx, Options{Quick: true, SkipDaemon: true})
	if len(rep.Results) == 0 {
		t.Fatal("expected results")
	}
	for _, res := range rep.Results {
		if res.Severity == SeverityFail {
			t.Fatalf("unexpected fail %s: %s", res.Name, res.Detail)
		}
		if strings.HasPrefix(res.Name, "network/") {
			t.Fatalf("quick mode should skip network: %s", res.Name)
		}
		if res.Name == "daemon/sandbox-smoke" {
			t.Fatalf("quick mode should skip daemon")
		}
	}
}

func TestRunDefaultNoDaemon(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	rep := Run(ctx, Options{SkipDaemon: true, Full: true})
	_, _, _, fail := rep.Counts()
	if fail > 0 {
		var b bytes.Buffer
		_ = rep.FormatText(&b)
		t.Fatalf("failures:\n%s", b.String())
	}
}
