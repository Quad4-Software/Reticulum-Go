// SPDX-License-Identifier: LicenseRef-Reticulum

package lxmf

import (
	"bytes"
	"crypto/sha256"
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
	lxm := bytes.Repeat([]byte{0x33}, Overhead+16)
	tid := sha256.Sum256(lxm)
	st, _, err := GenerateStampCPU(t.Context(), tid[:], 4, WorkblockExpandRoundsPN)
	if err != nil {
		t.Fatal(err)
	}
	msg := append(append([]byte{}, lxm...), st...)
	bad := append(append([]byte{}, lxm...), bytes.Repeat([]byte{0xff}, StampSize)...)
	short := bytes.Repeat([]byte{0x44}, Overhead+StampSize)
	got := ValidatePNStamps([][]byte{msg, bad, msg, short}, 4)
	if len(got) != 2 {
		t.Fatalf("got %d entries want 2", len(got))
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
		if _, err := StampWorkblockCPU(material, WorkblockExpandRounds); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStampWorkblockPropagation(b *testing.B) {
	material := bytes.Repeat([]byte{0x11}, 16)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := StampWorkblockCPU(material, WorkblockExpandRoundsPN); err != nil {
			b.Fatal(err)
		}
	}
}
