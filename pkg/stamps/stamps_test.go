// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package stamps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestTABDigests(t *testing.T) {
	if len(tab) != costFP {
		t.Fatalf("len(tab) = %d, want %d", len(tab), costFP)
	}
	if tab[0] != 1<<mantissaBits {
		t.Fatalf("tab[0] = %#x, want %#x", tab[0], 1<<mantissaBits)
	}
	for i := 1; i < len(tab); i++ {
		if tab[i] <= tab[i-1] {
			t.Fatalf("tab not strictly increasing at %d", i)
		}
	}

	parts := make([]string, len(tab))
	for i, v := range tab {
		parts[i] = fmt.Sprintf("%d", v)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, ",")))
	if got := hex.EncodeToString(sum[:]); got != tabDigest {
		t.Fatalf("TAB digest = %s, want %s", got, tabDigest)
	}

	prm := sha256.Sum256([]byte(fmt.Sprintf("master|%s|%d|%d|%d|%d|", tabDigest, mantissaBits, len(tab), costFPBits, costFP)))
	if got := hex.EncodeToString(prm[:]); got != prmDigest {
		t.Fatalf("PRM digest = %s, want %s", got, prmDigest)
	}
}

func TestCostFixedPointRoundTrip(t *testing.T) {
	for _, cost := range []float64{0, 1, 9.3691, 14.125, 23.5, 25, 99.99, 100} {
		b := CostToBytes(cost)
		got := CostFromBytes(b[:])
		if q := QuantizeCost(cost); got != q {
			t.Fatalf("CostFromBytes(CostToBytes(%v)) = %v, want %v", cost, got, q)
		}
	}
	if CostToFP(-5) != 0 {
		t.Fatal("negative cost must clamp to 0")
	}
	if CostToFP(500) != StampCostCap*costFP {
		t.Fatal("cost above cap must clamp")
	}
}

func TestGenerateStampValid(t *testing.T) {
	material := []byte("stamps-test-material")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stamp, value, err := GenerateStamp(ctx, material, 8, StampSize)
	if err != nil {
		t.Fatalf("GenerateStamp: %v", err)
	}
	if len(stamp) != StampSize {
		t.Fatalf("stamp size = %d", len(stamp))
	}
	if !StampValid(stamp, 8, material) {
		t.Fatal("generated stamp does not validate")
	}
	if value < 8 {
		t.Fatalf("stamp value %v below cost 8", value)
	}
}

func TestGenerateStampedKeyValid(t *testing.T) {
	material := []byte("stamps-key-material")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	priv, pub, value, err := GenerateStampedKey(ctx, material, 8)
	if err != nil {
		t.Fatalf("GenerateStampedKey: %v", err)
	}
	if priv == [32]byte{} {
		t.Fatal("empty private key")
	}
	if !StampValid(pub[:], 8, material) {
		t.Fatal("stamped key does not validate")
	}
	if value < 8 {
		t.Fatalf("stamped key value %v below cost 8", value)
	}
}

func TestDestinationStampStoreLoad(t *testing.T) {
	dir := t.TempDir()
	destHash := []byte("0123456789abcdef")
	material := []byte("stamp-material")
	stamp, value, err := GenerateStamp(context.Background(), material, 8, StampLength)
	if err != nil {
		t.Fatalf("GenerateStamp: %v", err)
	}
	if err := StoreDestinationStamp(dir, destHash, stamp, value); err != nil {
		t.Fatalf("StoreDestinationStamp: %v", err)
	}
	got, gotValue, ok := LoadDestinationStamp(dir, destHash, material, 8)
	if !ok {
		t.Fatal("LoadDestinationStamp failed")
	}
	if string(got) != string(stamp) || gotValue != value {
		t.Fatal("loaded stamp mismatch")
	}
	if _, _, ok := LoadDestinationStamp(dir, destHash, material, value+10); ok {
		t.Fatal("stamp should fail revalidation above its value")
	}
}

func TestStampWorkblockNoExpand(t *testing.T) {
	material := []byte("material")
	wb, err := StampWorkblock(material, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(wb) != string(material) {
		t.Fatal("expand_rounds=0 must return material")
	}
}
