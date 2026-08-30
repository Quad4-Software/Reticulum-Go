// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package lxstamper

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestGenerateStampCPUValid(t *testing.T) {
	msg := bytes.Repeat([]byte{0x11}, 16)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stamp, value, err := GenerateStampCPU(ctx, msg, 4, 3)
	if err != nil {
		t.Fatalf("cpu generate: %v", err)
	}
	wb, err := StampWorkblock(msg, 3)
	if err != nil {
		t.Fatal(err)
	}
	if value < 4 || !MeetsCost(stamp, 4, wb) {
		t.Fatalf("cpu stamp invalid value=%d", value)
	}
}

func TestGenerateStampGPUOrSkip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	msg := bytes.Repeat([]byte{0x22}, 16)
	stamp, value, err := GenerateStampGPU(ctx, msg, 4, 3)
	if err != nil {
		t.Skipf("GPU unavailable: %v", err)
	}
	wb, _ := StampWorkblock(msg, 3)
	if value < 4 || !MeetsCost(stamp, 4, wb) {
		t.Fatalf("gpu stamp invalid value=%d", value)
	}
	vendor, name, ok := GPUDeviceInfo()
	if !ok {
		t.Fatal("expected GPU device info")
	}
	t.Logf("GPU stamp ok on %s %s", vendor, name)
}

func TestGenerateStampAutoFallback(t *testing.T) {
	t.Setenv("RNS_LXSTAMP_BACKEND", "auto")
	msg := make([]byte, 16)
	rand.Read(msg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stamp, _, err := GenerateStamp(ctx, msg, 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	wb, _ := StampWorkblock(msg, 3)
	if !MeetsCost(stamp, 4, wb) {
		t.Fatal("auto backend stamp invalid")
	}
}

func BenchmarkGenerateStampCPU(b *testing.B) {
	msg := bytes.Repeat([]byte{0xAB}, 16)
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := GenerateStampCPU(ctx, msg, 8, DiscoveryRounds)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenerateStampGPU(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := GenerateStampGPU(ctx, []byte("probe"), 4, 3); err != nil {
		b.Skipf("GPU unavailable: %v", err)
	}
	msg := bytes.Repeat([]byte{0xCD}, 16)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := GenerateStampGPU(context.Background(), msg, 8, DiscoveryRounds)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenerateStampPython(b *testing.B) {
	py := os.Getenv("PYTHON_INTEROP")
	if py == "" {
		py = "python3"
	}
	if os.Getenv("RUN_PY_INTEROP") == "" {
		b.Skip("set RUN_PY_INTEROP=1")
	}
	if _, err := exec.LookPath(py); err != nil {
		b.Skip(err)
	}
	script := `
import os, sys, base64, time
lxmf = os.environ.get("LXMF_PATH", "")
if lxmf:
    sys.path.insert(0, os.path.abspath(lxmf))
from LXMF import LXStamper
material = base64.b64decode(sys.argv[1])
cost = int(sys.argv[2])
rounds = int(sys.argv[3])
st = time.perf_counter()
stamp, value = LXStamper.generate_stamp(material, cost, expand_rounds=rounds)
dt = time.perf_counter() - st
sys.stdout.write(f"{dt}\n")
`
	msg := bytes.Repeat([]byte{0xEF}, 16)
	b64 := base64.StdEncoding.EncodeToString(msg)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(py, "-c", script, b64, "8", fmt.Sprintf("%d", DiscoveryRounds))
		if p := os.Getenv("LXMF_PATH"); p != "" {
			cmd.Env = append(os.Environ(), "LXMF_PATH="+p)
		}
		out, err := cmd.Output()
		if err != nil {
			b.Fatal(err)
		}
		_ = out
	}
}
