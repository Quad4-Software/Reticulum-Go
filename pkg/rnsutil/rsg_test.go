// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/identity"
)

func TestRSGRoundTripAndPythonFixture(t *testing.T) {
	dir := t.TempDir()
	id, err := GenerateIdentity(filepath.Join(dir, "id.rid"))
	if err != nil {
		t.Fatal(err)
	}
	msgPath := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(msgPath, []byte("hello reticulum go"), 0o600); err != nil {
		t.Fatal(err)
	}
	rsg, err := SignFileRSG(id, msgPath)
	if err != nil {
		t.Fatal(err)
	}
	res, err := VerifyFileRSG(rsg, msgPath, id)
	if err != nil || !res.Valid {
		t.Fatalf("verify: valid=%v err=%v", res.Valid, err)
	}

	rsm, err := CreateRSM(id, "signed message body", nil)
	if err != nil {
		t.Fatal(err)
	}
	res2, text, err := VerifyRSM(rsm, id)
	if err != nil || !res2.Valid || text != "signed message body" {
		t.Fatalf("rsm: valid=%v text=%q err=%v", res2.Valid, text, err)
	}
}

func TestPythonRSGFixture(t *testing.T) {
	base := filepath.Join("testdata", "python_rsg")
	id, err := LoadIdentity(filepath.Join(base, "id.rid"))
	if err != nil {
		t.Fatal(err)
	}
	rsg, err := os.ReadFile(filepath.Join(base, "hello.txt.rsg"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := VerifyFileRSG(rsg, filepath.Join(base, "hello.txt"), id)
	if err != nil || !res.Valid {
		t.Fatalf("python rsg: valid=%v err=%v", res.Valid, err)
	}
	if !bytes.Equal(res.Signer.Hash(), id.Hash()) {
		t.Fatal("signer hash mismatch")
	}

	rsm, err := os.ReadFile(filepath.Join(base, "msg.rsm"))
	if err != nil {
		t.Fatal(err)
	}
	res2, text, err := VerifyRSM(rsm, id)
	if err != nil || !res2.Valid {
		t.Fatalf("python rsm: valid=%v err=%v", res2.Valid, err)
	}
	if text != "signed message body" {
		t.Fatalf("message=%q", text)
	}
}

func TestPythonRFEFixture(t *testing.T) {
	base := filepath.Join("testdata", "python_rsg")
	id, err := LoadIdentity(filepath.Join(base, "id.rid"))
	if err != nil {
		t.Fatal(err)
	}
	ct, err := os.ReadFile(filepath.Join(base, "cipher.rfe"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := id.Decrypt(ct, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(base, "plain.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pt, want) {
		t.Fatalf("plaintext mismatch %d vs %d", len(pt), len(want))
	}
}

func TestRFERoundTrip(t *testing.T) {
	dir := t.TempDir()
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	inPath := filepath.Join(dir, "plain.txt")
	encPath := filepath.Join(dir, "plain.txt.rfe")
	outPath := filepath.Join(dir, "plain.out")
	payload := bytes.Repeat([]byte("secret payload for rfe test "), 10)
	if err := os.WriteFile(inPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EncryptFileRFE(id, inPath, encPath); err != nil {
		t.Fatal(err)
	}
	if err := DecryptFileRFE(id, encPath, outPath); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("rfe roundtrip mismatch")
	}
}

func TestGoRSGReadableByPythonStructure(t *testing.T) {
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rsg, err := CreateRSG(id, []byte("cross"), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rsg) <= SignatureSize {
		t.Fatal("rsg too short")
	}
	res, err := ValidateRSG(rsg, []byte("cross"), id.Hash())
	if err != nil || !res.Valid {
		t.Fatalf("valid=%v err=%v", res.Valid, err)
	}
}
