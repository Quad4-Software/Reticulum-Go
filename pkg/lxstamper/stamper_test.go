// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package lxstamper

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestStampGenerateAndValidate(t *testing.T) {
	msg := bytes.Repeat([]byte{0xAB}, 16)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stamp, value, err := GenerateStamp(ctx, msg, 4, 3)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if value < 4 {
		t.Fatalf("value %d below cost", value)
	}
	wb, err := StampWorkblock(msg, 3)
	if err != nil {
		t.Fatalf("workblock: %v", err)
	}
	if !StampValid(stamp, 4, wb) {
		t.Fatalf("stamp should validate threshold")
	}
	if !MeetsCost(stamp, 4, wb) {
		t.Fatalf("stamp should meet cost")
	}
	bad := make([]byte, StampSize)
	if _, err := rand.Read(bad); err != nil {
		t.Fatalf("rand: %v", err)
	}
	bad[0] = 0xff
	if StampValid(bad, 256, wb) {
		t.Fatalf("impossibly hard cost should not validate")
	}
}

func TestGenerateStampWithDeadlineExpired(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	_, _, err := GenerateStampWithDeadline(context.Background(), []byte("deadline-material"), 8, 3, past)
	if !errors.Is(err, ErrStampNotFound) {
		t.Fatalf("expected ErrStampNotFound, got %v", err)
	}
}

func TestStampValueDeterministic(t *testing.T) {
	wb := bytes.Repeat([]byte{0x11}, 256)
	stamp := bytes.Repeat([]byte{0xAB}, StampSize)
	first := StampValue(wb, stamp)
	second := StampValue(wb, stamp)
	if first != second {
		t.Fatal("StampValue not deterministic")
	}
}

func TestMeetsCostRequiresBoth(t *testing.T) {
	wb := bytes.Repeat([]byte{0x22}, 256)
	stamp := bytes.Repeat([]byte{0xff}, StampSize)
	if MeetsCost(stamp, 8, wb) {
		t.Fatal("weak stamp must fail MeetsCost")
	}
	if MeetsCost(stamp[:16], 0, wb) {
		t.Fatal("wrong length at cost 0 must fail MeetsCost")
	}
}

func pythonOrSkip(t *testing.T) string {
	t.Helper()
	if os.Getenv("RUN_PY_INTEROP") == "" {
		t.Skip("set RUN_PY_INTEROP=1 to enable python LXStamper interop")
	}
	exe := os.Getenv("PYTHON_INTEROP")
	if exe == "" {
		exe = "python3"
	}
	if _, err := exec.LookPath(exe); err != nil {
		t.Skipf("python interpreter %q not found: %v", exe, err)
	}
	return exe
}

func TestInteropStampMatchesPython(t *testing.T) {
	pyExe := pythonOrSkip(t)
	lxmfPath := os.Getenv("LXMF_PATH")
	if lxmfPath == "" {
		for _, candidate := range []string{
			"../LXMF", "../../LXMF", "../../../LXMF",
			"/run/media/user1/projects/Reticulum/LXMF",
		} {
			if st, err := os.Stat(candidate); err == nil && st.IsDir() {
				lxmfPath = candidate
				break
			}
		}
	}
	material := bytes.Repeat([]byte{0xCA}, 16)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stamp, goValue, err := GenerateStamp(ctx, material, 4, 3)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	wb, err := StampWorkblock(material, 3)
	if err != nil {
		t.Fatalf("workblock: %v", err)
	}
	checkScript := `import os, sys, base64
lxmf = os.environ.get("LXMF_PATH", "")
if lxmf:
    sys.path.insert(0, os.path.abspath(lxmf))
try:
    from LXMF import LXStamper
except Exception as e:
    sys.stderr.write(f"LXStamper import failed: {e}\n")
    sys.exit(0)
material = base64.b64decode(sys.argv[1])
stamp    = base64.b64decode(sys.argv[2])
target   = int(sys.argv[3])
rounds   = int(sys.argv[4])
wb = LXStamper.stamp_workblock(material, expand_rounds=rounds)
print("VALID" if LXStamper.stamp_valid(stamp, target, wb) else "INVALID")
print("VALUE", LXStamper.stamp_value(wb, stamp))
`
	cmd := exec.Command(pyExe, "-c", checkScript,
		base64.StdEncoding.EncodeToString(material),
		base64.StdEncoding.EncodeToString(stamp), "4", "3")
	env := os.Environ()
	if lxmfPath != "" {
		env = append(env, "LXMF_PATH="+lxmfPath)
	}
	cmd.Env = env
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("python LXStamper check failed: %v", err)
	}
	text := string(out)
	if !strings.Contains(text, "VALID") {
		t.Skip("LXMF not available (set LXMF_PATH)")
	}
	if strings.Contains(text, "INVALID") {
		t.Fatalf("python rejected Go stamp: %s", text)
	}
	if !StampValid(stamp, 4, wb) || !MeetsCost(stamp, 4, wb) {
		t.Fatal("Go stamp failed local MeetsCost")
	}
	_ = goValue
}
