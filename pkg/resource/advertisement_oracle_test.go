// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package resource

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOracleResourceConstantsMatchPythonRNS(t *testing.T) {
	if Window != 4 {
		t.Fatalf("WINDOW=%d want 4", Window)
	}
	if WindowMin != 2 {
		t.Fatalf("WINDOW_MIN=%d want 2", WindowMin)
	}
	if WindowMaxSlow != 10 {
		t.Fatalf("WINDOW_MAX_SLOW=%d want 10", WindowMaxSlow)
	}
	if WindowMaxVerySlow != 4 {
		t.Fatalf("WINDOW_MAX_VERY_SLOW=%d want 4", WindowMaxVerySlow)
	}
	if WindowMaxFast != 75 {
		t.Fatalf("WINDOW_MAX_FAST=%d want 75", WindowMaxFast)
	}
	if WindowFlexibility != 4 {
		t.Fatalf("WINDOW_FLEXIBILITY=%d want 4", WindowFlexibility)
	}
	if MapHashLen != 4 {
		t.Fatalf("MAPHASH_LEN=%d want 4", MapHashLen)
	}
	if Overhead != 134 {
		t.Fatalf("OVERHEAD=%d want 134", Overhead)
	}
	if DefaultLinkMDU != 431 {
		t.Fatalf("Link.MDU=%d want 431", DefaultLinkMDU)
	}
	if HashmapMaxLen != 74 {
		t.Fatalf("HASHMAP_MAX_LEN=%d want 74", HashmapMaxLen)
	}
	if CollisionGuardSize != 224 {
		t.Fatalf("COLLISION_GUARD_SIZE=%d want 224", CollisionGuardSize)
	}
	if MaxEfficientSize != 1*1024*1024-1 {
		t.Fatalf("MAX_EFFICIENT_SIZE=%d want 1048575", MaxEfficientSize)
	}
	if HashmapEntriesPerSegment(DefaultLinkMDU) != HashmapMaxLen {
		t.Fatalf("HashmapEntriesPerSegment(%d)=%d want %d", DefaultLinkMDU, HashmapEntriesPerSegment(DefaultLinkMDU), HashmapMaxLen)
	}
	if FastRateThreshold != 4 {
		t.Fatalf("FAST_RATE_THRESHOLD=%d want WINDOW_MAX_SLOW-WINDOW-2=4", FastRateThreshold)
	}
}

func TestOracleResourceAdvertisementFlagsMatchPython(t *testing.T) {
	want := byte(0)
	want |= AdvFlagEncrypted
	want |= AdvFlagCompressed
	want |= AdvFlagSplit
	want |= AdvFlagHasMetadata
	if want != 0x27 {
		t.Fatalf("flag packing=%#02x want 0x27", want)
	}
	ra := &ResourceAdvertisement{Flags: want}
	ra.Encrypted = ra.Flags&AdvFlagEncrypted != 0
	ra.Compressed = ra.Flags&AdvFlagCompressed != 0
	ra.Split = ra.Flags&AdvFlagSplit != 0
	ra.IsRequest = ra.Flags&AdvFlagIsRequest != 0
	ra.IsResponse = ra.Flags&AdvFlagIsResponse != 0
	ra.HasMetadata = ra.Flags&AdvFlagHasMetadata != 0
	if !ra.Encrypted || !ra.Compressed || !ra.Split || ra.IsRequest || ra.IsResponse || !ra.HasMetadata {
		t.Fatalf("decoded flags %+v", ra)
	}
}

func TestOraclePythonResourceAdvertisementUnpacks(t *testing.T) {
	rawJSON, err := os.ReadFile(filepath.Join("..", "packet", "testdata", "rns_wire_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		ResourceAdvHex string `json:"resource_adv_hex"`
	}
	if err := json.Unmarshal(rawJSON, &file); err != nil {
		t.Fatal(err)
	}
	blob, err := hex.DecodeString(file.ResourceAdvHex)
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnpackResourceAdvertisement(blob)
	if err != nil {
		t.Fatalf("unpack python advertisement: %v", err)
	}
	if got.TransferSize != 1024 || got.DataSize != 2048 || got.Parts != 3 {
		t.Fatalf("sizes t=%d d=%d n=%d", got.TransferSize, got.DataSize, got.Parts)
	}
	if got.Flags != 0x27 {
		t.Fatalf("flags=%#02x want 0x27", got.Flags)
	}
	if !got.Encrypted || !got.Compressed || !got.Split || !got.HasMetadata {
		t.Fatalf("flag booleans encrypted=%v compressed=%v split=%v meta=%v", got.Encrypted, got.Compressed, got.Split, got.HasMetadata)
	}
	if !bytes.Equal(got.Hash, bytesSeq(32)) {
		t.Fatalf("hash mismatch")
	}
	if !bytes.Equal(got.Hashmap, []byte{0x11, 0x22, 0x33, 0x44}) {
		t.Fatalf("hashmap=%x", got.Hashmap)
	}
}

func bytesSeq(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

func TestPythonRNSUnpacksGoResourceAdvertisement(t *testing.T) {
	exe := os.Getenv("PYTHON_INTEROP")
	if exe == "" {
		exe = "python3"
	}
	ra := &ResourceAdvertisement{
		TransferSize:  1024,
		DataSize:      2048,
		Parts:         3,
		Hash:          bytesSeq(32),
		RandomHash:    []byte{1, 2, 3, 4},
		OriginalHash:  bytesSeq32(32),
		Hashmap:       []byte{0x11, 0x22, 0x33, 0x44},
		Flags:         0x27,
		SegmentIndex:  0,
		TotalSegments: 1,
	}
	packed, err := ra.Pack(0, DefaultLinkMDU)
	if err != nil {
		t.Fatal(err)
	}
	script := `
from RNS.Resource import ResourceAdvertisement
import sys
adv = ResourceAdvertisement.unpack(bytes.fromhex(sys.argv[1]))
print(adv.t, adv.d, adv.n, adv.f, int(adv.e), int(adv.c), int(adv.s), int(adv.x), adv.m.hex())
`
	cmd := exec.Command(exe, "-c", script, hex.EncodeToString(packed))
	out, err := cmd.CombinedOutput()
	if err != nil {
		if os.Getenv("RUN_PY_INTEROP") != "" {
			t.Fatalf("python ResourceAdvertisement required: %v\n%s", err, out)
		}
		t.Skip("python RNS not available")
	}
	got := strings.TrimSpace(string(out))
	want := "1024 2048 3 39 1 1 1 1 11223344"
	if got != want {
		t.Fatalf("python unpack %q want %q", got, want)
	}
}

func bytesSeq32(start int) []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(start + i)
	}
	return b
}
