// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"testing"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
)

func TestPackParseRNXRequestRoundTrip(t *testing.T) {
	to := 15.0
	ol, el := 100, 50
	req := RNXRequest{
		Command:     `echo "hi there"`,
		TimeoutSec:  &to,
		StdoutLimit: &ol,
		StderrLimit: &el,
		Stdin:       []byte("stdin-data"),
	}
	packed, err := msgpack.Marshal(PackRNXRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseRNXRequestPayload(packed)
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != req.Command {
		t.Fatalf("command=%q", got.Command)
	}
	if got.TimeoutSec == nil || *got.TimeoutSec != to {
		t.Fatalf("timeout=%v", got.TimeoutSec)
	}
	if got.StdoutLimit == nil || *got.StdoutLimit != ol {
		t.Fatalf("stdout=%v", got.StdoutLimit)
	}
	if !bytes.Equal(got.Stdin, req.Stdin) {
		t.Fatalf("stdin=%q", got.Stdin)
	}
}

func TestPackParseRNXResultRoundTrip(t *testing.T) {
	code := 7
	concluded := 100.5
	res := RNXResult{
		Executed:    true,
		ReturnCode:  &code,
		Stdout:      []byte("out"),
		Stderr:      []byte("err"),
		StdoutTotal: 3,
		StderrTotal: 3,
		StartedAt:   100.0,
		ConcludedAt: &concluded,
	}
	raw := PackRNXResult(res)
	got, err := ParseRNXResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Executed || got.ReturnCode == nil || *got.ReturnCode != 7 {
		t.Fatalf("%+v", got)
	}
	if string(got.Stdout) != "out" || string(got.Stderr) != "err" {
		t.Fatalf("io %+v", got)
	}
}

func TestSplitShellCommand(t *testing.T) {
	args, err := SplitShellCommand(`echo "a b" 'c d'`)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 3 || args[0] != "echo" || args[1] != "a b" || args[2] != "c d" {
		t.Fatalf("%v", args)
	}
}

func TestExecuteRNXCommandLocally(t *testing.T) {
	to := 5.0
	res := ExecuteRNXCommandLocally(RNXRequest{
		Command:    "echo hello-rnx",
		TimeoutSec: &to,
	})
	if !res.Executed {
		t.Fatal("not executed")
	}
	if res.ReturnCode == nil || *res.ReturnCode != 0 {
		t.Fatalf("code=%v", res.ReturnCode)
	}
	if !bytes.Contains(res.Stdout, []byte("hello-rnx")) {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestExecuteRNXCommandLocallyTimeout(t *testing.T) {
	to := 0.2
	start := time.Now()
	res := ExecuteRNXCommandLocally(RNXRequest{
		Command:    "sleep 5",
		TimeoutSec: &to,
	})
	if time.Since(start) > 3*time.Second {
		t.Fatal("timeout too slow")
	}
	if !res.Executed {
		t.Fatal("expected start")
	}
	if res.ConcludedAt != nil {
		t.Fatal("timed out commands should leave concluded nil")
	}
}

func TestRNXRequestTimeout(t *testing.T) {
	d := RNXRequestTimeout(15*time.Second, 0.1)
	want := 15*time.Second + 400*time.Millisecond + RNXRemoteExecGrace
	if d != want {
		t.Fatalf("got %v want %v", d, want)
	}
}

func TestTruncateBytes(t *testing.T) {
	zero := 0
	lim := 2
	if len(truncateBytes([]byte("abcd"), &zero)) != 0 {
		t.Fatal("zero")
	}
	if string(truncateBytes([]byte("abcd"), &lim)) != "ab" {
		t.Fatal("lim")
	}
	if string(truncateBytes([]byte("ab"), nil)) != "ab" {
		t.Fatal("nil")
	}
}

func FuzzParseRNXRequestPayload(f *testing.F) {
	to := 1.0
	seed, _ := msgpack.Marshal(PackRNXRequest(RNXRequest{Command: "true", TimeoutSec: &to}))
	f.Add(seed)
	f.Add([]byte{0x90})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseRNXRequestPayload(data)
	})
}

func FuzzParseRNXResult(f *testing.F) {
	code := 0
	seedList := PackRNXResult(RNXResult{Executed: true, ReturnCode: &code, StartedAt: 1})
	seed, _ := msgpack.Marshal(seedList)
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseRNXResult(data)
		_, _ = ParseRNXResult(any(data))
	})
}

func FuzzSplitShellCommand(f *testing.F) {
	f.Add(`echo hi`)
	f.Add(`echo "a b"`)
	f.Add(`'x'`)
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = SplitShellCommand(s)
	})
}
