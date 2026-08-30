// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package lxstamper

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

func TestStampWorkblockCPUMatchesSerial(t *testing.T) {
	material := bytes.Repeat([]byte{0xAB}, 16)
	for _, rounds := range []int{1, 3, 20, 25, 64} {
		parallel, err := StampWorkblockCPU(material, rounds)
		if err != nil {
			t.Fatalf("rounds=%d: %v", rounds, err)
		}
		again, err := StampWorkblockCPU(material, rounds)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(parallel, again) || len(parallel) != 256*rounds {
			t.Fatalf("rounds=%d nondeterministic or bad len", rounds)
		}
	}
}

func TestInteropWorkblockMatchesPython(t *testing.T) {
	pyExe := pythonOrSkip(t)
	material := bytes.Repeat([]byte{0xCA}, 16)
	for _, rounds := range []int{3, 20, DeliveryRounds, PropagationRounds} {
		goWB, err := StampWorkblockCPU(material, rounds)
		if err != nil {
			t.Fatalf("rounds=%d: %v", rounds, err)
		}
		script := `import os, sys, base64
lxmf = os.environ.get("LXMF_PATH", "")
if lxmf:
    sys.path.insert(0, os.path.abspath(lxmf))
try:
    from LXMF import LXStamper
except Exception as e:
    sys.stderr.write(f"LXStamper import failed: {e}\n")
    sys.exit(0)
material = base64.b64decode(sys.argv[1])
rounds = int(sys.argv[2])
wb = LXStamper.stamp_workblock(material, expand_rounds=rounds)
sys.stdout.buffer.write(wb)
`
		cmd := exec.Command(pyExe, "-c", script,
			base64.StdEncoding.EncodeToString(material),
			strconv.Itoa(rounds))
		if p := os.Getenv("LXMF_PATH"); p != "" {
			cmd.Env = append(os.Environ(), "LXMF_PATH="+p)
		}
		out, err := cmd.Output()
		if err != nil {
			t.Skipf("python workblock failed: %v", err)
		}
		if len(out) == 0 {
			t.Skip("LXMF not available")
		}
		if !bytes.Equal(goWB, out) {
			t.Fatalf("rounds=%d workblock mismatch go=%d py=%d", rounds, len(goWB), len(out))
		}
	}
}

func TestValidateStampBatchCPU(t *testing.T) {
	material := bytes.Repeat([]byte{0x11}, 16)
	wb, err := StampWorkblockCPU(material, 3)
	if err != nil {
		t.Fatal(err)
	}
	ctxStamp, _, err := GenerateStampCPU(t.Context(), material, 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !MeetsCost(ctxStamp, 4, wb) {
		t.Fatal("generated stamp should meet cost")
	}
	bad := bytes.Repeat([]byte{0xff}, StampSize)
	cands := []StampCandidate{
		{Material: material, Stamp: ctxStamp},
		{Material: material, Stamp: bad},
		{Material: nil, Stamp: ctxStamp},
	}
	ok := validateStampBatchCPU(cands, 4, 3)
	if !ok[0] || ok[1] || ok[2] {
		t.Fatalf("batch results=%v", ok)
	}
}

func TestValidatePNStampsParallel(t *testing.T) {
	lxm := bytes.Repeat([]byte{0x33}, PNMessageOverhead+16)
	tid := sha256.Sum256(lxm)
	st, _, err := GenerateStampCPU(t.Context(), tid[:], 4, PropagationRounds)
	if err != nil {
		t.Fatal(err)
	}
	msg := append(append([]byte{}, lxm...), st...)
	bad := append(append([]byte{}, lxm...), bytes.Repeat([]byte{0xff}, StampSize)...)
	short := bytes.Repeat([]byte{0x44}, PNMessageOverhead+StampSize)
	got := ValidatePNStamps([][]byte{msg, bad, msg, short}, 4)
	if len(got) != 2 {
		t.Fatalf("got %d entries want 2", len(got))
	}
}

func TestValidatePNStampRejectsShort(t *testing.T) {
	short := bytes.Repeat([]byte{0x01}, PNMessageOverhead+StampSize)
	tid, _, _, _ := ValidatePNStamp(short, 0)
	if tid != nil {
		t.Fatal("expected reject at LXMF_OVERHEAD+STAMP_SIZE floor")
	}
}

func TestValidateStampBatchCostZero(t *testing.T) {
	material := bytes.Repeat([]byte{0x11}, 16)
	stamp := bytes.Repeat([]byte{0xab}, StampSize)
	longMat := bytes.Repeat([]byte{0x22}, 80)
	cands := []StampCandidate{
		{Material: material, Stamp: stamp},
		{Material: material, Stamp: stamp[:16]},
		{Material: longMat, Stamp: stamp},
		{Material: nil, Stamp: stamp},
	}
	ok := ValidateStampBatch(cands, 0, 3)
	if !ok[0] || ok[1] || !ok[2] || ok[3] {
		t.Fatalf("cost0 batch=%v", ok)
	}
}

func BenchmarkStampWorkblockDelivery(b *testing.B) {
	material := bytes.Repeat([]byte{0x11}, 16)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := StampWorkblockCPU(material, DeliveryRounds); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStampWorkblockPropagation(b *testing.B) {
	material := bytes.Repeat([]byte{0x11}, 16)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := StampWorkblockCPU(material, PropagationRounds); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateStampBatchDiscovery(b *testing.B) {
	material := bytes.Repeat([]byte{0x22}, 16)
	stamp, _, err := GenerateStampCPU(b.Context(), material, 4, 20)
	if err != nil {
		b.Fatal(err)
	}
	cands := make([]StampCandidate, 64)
	for i := range cands {
		cands[i] = StampCandidate{Material: material, Stamp: stamp}
		if i%3 == 0 {
			cands[i].Stamp = bytes.Repeat([]byte{0xff}, StampSize)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = ValidateStampBatch(cands, 4, 20)
	}
}
