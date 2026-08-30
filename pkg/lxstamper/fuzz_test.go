// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package lxstamper

import (
	"bytes"
	"testing"
)

func FuzzStampWorkblock(f *testing.F) {
	f.Add([]byte{0x01, 0x02, 0x03, 0x04}, 1)
	f.Add(bytes.Repeat([]byte{0xab}, 16), 3)
	f.Add(bytes.Repeat([]byte{0xcd}, 32), 20)
	f.Add(bytes.Repeat([]byte{0xef}, 80), 2)
	f.Fuzz(func(t *testing.T, material []byte, rounds int) {
		if len(material) == 0 || len(material) > 256 || rounds < 1 || rounds > 64 {
			t.Skip()
		}
		a, err := StampWorkblockCPU(material, rounds)
		if err != nil {
			t.Fatalf("workblock: %v", err)
		}
		if len(a) != 256*rounds {
			t.Fatalf("len=%d want %d", len(a), 256*rounds)
		}
		b, err := StampWorkblockCPU(material, rounds)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Fatal("nondeterministic workblock")
		}
	})
}

func FuzzStampValidMeetsCost(f *testing.F) {
	f.Add(bytes.Repeat([]byte{0x00}, StampSize), bytes.Repeat([]byte{0x00}, 256), 0)
	f.Add(bytes.Repeat([]byte{0xff}, StampSize), bytes.Repeat([]byte{0xaa}, 512), 4)
	f.Add([]byte{0x01}, []byte{0x02}, -1)
	f.Add(bytes.Repeat([]byte{0x11}, StampSize), bytes.Repeat([]byte{0x22}, 256), 257)
	f.Fuzz(func(t *testing.T, stamp, workblock []byte, cost int) {
		if len(stamp) > 128 || len(workblock) > 1<<14 {
			t.Skip()
		}
		_ = StampValue(workblock, stamp)
		okValid := StampValid(stamp, cost, workblock)
		okMeet := MeetsCost(stamp, cost, workblock)
		if okMeet && !okValid && cost > 0 {
			t.Fatal("MeetsCost true while StampValid false")
		}
		if cost > 0 && len(stamp) != StampSize && (okValid || okMeet) {
			t.Fatal("wrong-length stamp accepted at positive cost")
		}
		if cost <= 0 && okMeet && len(stamp) != StampSize {
			t.Fatal("MeetsCost must require StampSize at cost<=0")
		}
	})
}

func FuzzValidateStampBatch(f *testing.F) {
	mat := bytes.Repeat([]byte{0x42}, 16)
	st := bytes.Repeat([]byte{0x7e}, StampSize)
	f.Add(mat, st, 0, 1)
	f.Add(mat, st, 4, 3)
	f.Add(bytes.Repeat([]byte{0x01}, 80), st, 0, 2)
	f.Fuzz(func(t *testing.T, material, stamp []byte, cost, rounds int) {
		if rounds < 1 || rounds > 8 || len(material) > 128 || len(stamp) > 64 {
			t.Skip()
		}
		cands := []StampCandidate{
			{Material: material, Stamp: stamp},
			{Material: material, Stamp: append([]byte(nil), stamp...)},
			{Material: nil, Stamp: stamp},
			{Material: material, Stamp: stamp[:min(len(stamp), 8)]},
		}
		got := ValidateStampBatch(cands, cost, rounds)
		if len(got) != len(cands) {
			t.Fatalf("len=%d", len(got))
		}
		ref := validateStampBatchCPU(cands, cost, rounds)
		for i := range got {
			if got[i] != ref[i] {
				t.Fatalf("batch[%d]=%v want %v", i, got[i], ref[i])
			}
		}
	})
}

func FuzzValidatePNStamp(f *testing.F) {
	f.Add([]byte{}, 0)
	f.Add(bytes.Repeat([]byte{0x01}, PNMessageOverhead+StampSize), 0)
	f.Add(bytes.Repeat([]byte{0x02}, 40), 4)
	f.Add(bytes.Repeat([]byte{0x03}, StampSize+1), -1)
	f.Fuzz(func(t *testing.T, transient []byte, cost int) {
		if len(transient) > PNMessageOverhead+StampSize {
			// Full PN validation expands 1000 HKDF rounds. Keep fuzz on the
			// cheap reject path only.
			t.Skip()
		}
		tid, lxm, _, stamp := ValidatePNStamp(transient, cost)
		if tid != nil || lxm != nil || stamp != nil {
			t.Fatal("short transient must be rejected")
		}
	})
}
