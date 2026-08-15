// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"encoding/hex"
	"testing"

	"quad4/reticulum-go/pkg/identity"
)

func TestParseDestHashExploratory(t *testing.T) {
	good := "00112233445566778899aabbccddeeff"
	b, err := ParseDestHash(good)
	if err != nil || len(b) != 16 {
		t.Fatalf("good: %v len=%d", err, len(b))
	}
	b2, err := ParseDestHash("<" + good + ">")
	if err != nil || !bytes.Equal(b, b2) {
		t.Fatalf("brackets: %v", err)
	}
	if _, err := ParseDestHash("short"); err == nil {
		t.Fatal("short accepted")
	}
	if _, err := ParseDestHash(good + "ff"); err == nil {
		t.Fatal("long accepted")
	}
	if _, err := ParseDestHash("zz112233445566778899aabbccddeeff"); err == nil {
		t.Fatal("non-hex accepted")
	}
}

func TestValidateRSGRejectsGarbage(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		{0x01},
		bytes.Repeat([]byte{0xab}, SignatureSize),
		bytes.Repeat([]byte{0xab}, SignatureSize+1),
		append([]byte("-----BEGIN RNS SIGNATURE-----\n"), bytes.Repeat([]byte{0xab}, 80)...),
	}
	for i, raw := range cases {
		if _, err := ValidateRSG(raw, []byte("msg"), nil); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}

func TestValidateRSGRoundTripBitflip(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("exploratory-rsg-payload")
	rsg, err := CreateRSG(id, msg, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ValidateRSG(rsg, msg, id)
	if err != nil || !res.Valid {
		t.Fatalf("valid: err=%v valid=%v", err, res.Valid)
	}
	flipped := append([]byte(nil), rsg...)
	flipped[len(flipped)/2] ^= 0xff
	if _, err := ValidateRSG(flipped, msg, id); err == nil {
		t.Fatal("bitflip accepted")
	}
	wrong, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateRSG(rsg, msg, wrong); err == nil {
		t.Fatal("wrong signer accepted")
	}
}

// FuzzValidateRSGExploratory ensures arbitrary blobs never panic and never
// validate without a real signature envelope.
func FuzzValidateRSGExploratory(f *testing.F) {
	id, err := identity.New()
	if err != nil {
		f.Fatal(err)
	}
	msg := []byte("seed")
	if rsg, err := CreateRSG(id, msg, false, nil); err == nil {
		f.Add(rsg, msg)
	}
	f.Add([]byte{}, []byte("x"))
	f.Add([]byte{0xff, 0x00}, []byte{})
	f.Add(bytes.Repeat([]byte{0x11}, SignatureSize), []byte("m"))

	f.Fuzz(func(t *testing.T, rsg, message []byte) {
		if len(rsg) > 1<<16 || len(message) > 1<<14 {
			t.Skip()
		}
		res, err := ValidateRSG(rsg, message, nil)
		if err != nil {
			return
		}
		if !res.Valid {
			t.Fatal("nil error with Valid=false")
		}
		if res.Signer == nil {
			t.Fatal("valid result missing signer")
		}
	})
}

func TestParseDestHashHexLenExploratory(t *testing.T) {
	for n := range 40 {
		s := hex.EncodeToString(bytes.Repeat([]byte{0xab}, n))
		b, err := ParseDestHash(s)
		if n == 16 {
			if err != nil || len(b) != 16 {
				t.Fatalf("n=16: err=%v len=%d", err, len(b))
			}
			continue
		}
		if err == nil {
			t.Fatalf("n=%d accepted len=%d", n, len(b))
		}
	}
}
