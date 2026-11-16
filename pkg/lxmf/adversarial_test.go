// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

func TestAdversarial_TrailingJunkAfterStampedPayloadRejected(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)

	msg, err := NewMessage(dst.Hash(), src.Hash(), []byte("t"), []byte("body"), nil)
	if err != nil {
		t.Fatal(err)
	}
	msg.Timestamp = 1_800_000_000
	msg.Stamp = bytes.Repeat([]byte{0x11}, StampSize)
	raw := mustPack(t, msg, src)
	junked := append(append([]byte(nil), raw...), 0x00, 0xff)

	got, err := Unpack(junked, RecallSource)
	if err == nil {
		t.Fatalf("trailing junk accepted: validated=%v packed=%d", got.SignatureValidated, len(got.Packed))
	}
	if !errors.Is(err, ErrInvalidPayload) && !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestAdversarial_ExtraPayloadArrayElementsRejected(t *testing.T) {
	payload := []any{
		float64(1),
		[]byte("t"),
		[]byte("c"),
		map[byte]any{},
		bytes.Repeat([]byte{1}, StampSize),
		"extra",
	}
	blob, err := msgpack.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = decodePayloadAndSplit(blob)
	if err == nil {
		t.Fatal("6-element payload accepted")
	}
}

func TestAdversarial_PaperDecodeRejectsOverMDU(t *testing.T) {
	oversized := bytes.Repeat([]byte{0x01}, PaperMDU+1)
	enc := base64.URLEncoding.EncodeToString(oversized)
	enc = strings.TrimRight(enc, "=")
	uri := "lxm://" + enc
	got, err := DecodePaperURI(uri)
	if err == nil {
		t.Fatalf("accepted oversized paper decode len=%d", len(got))
	}
	if !errors.Is(err, ErrInvalidURI) {
		t.Fatalf("err=%v", err)
	}
}

func TestAdversarial_NegativeAnnounceStampCostRejected(t *testing.T) {
	raw, err := msgpack.Marshal([]any{[]byte("n"), int64(-1)})
	if err != nil {
		t.Fatal(err)
	}
	cost, ok, err := StampCostFromAppData(raw)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("negative cost accepted: %d", cost)
	}
}

func TestAdversarial_ShortPoWStampRejected(t *testing.T) {
	wb := bytes.Repeat([]byte{0x22}, 256)
	if StampValid([]byte{0x01}, 8, wb) {
		t.Fatal("short stamp must fail when targetCost>0")
	}
	if StampValid(bytes.Repeat([]byte{0x01}, StampSize), 8, wb) {
		// Unlikely but ok if random happens to satisfy; only length is required here.
	}
}

func TestAdversarial_ValidateStampRequiresHash(t *testing.T) {
	m := &LXMessage{Stamp: bytes.Repeat([]byte{1}, StampSize)}
	if _, err := m.ValidateStamp(8, nil); err == nil {
		t.Fatal("expected error without hash")
	}
}

func TestAdversarial_MutationAppendNoiseMustError(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)
	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)
	msg, err := NewMessage(dst.Hash(), src.Hash(), nil, []byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	base := mustPack(t, msg, src)
	mut := append(append([]byte(nil), base...), 0xaa)
	if _, err := Unpack(mut, RecallSource); err == nil {
		t.Fatal("appended noise must not verify")
	}
}

func TestAdversarial_PNNegativeCostNotOK(t *testing.T) {
	blob := buildPNAnnounce(t, 1, true, 256, 10240, -1, 3, 18, nil)
	cost, ok, err := PNStampCostFromAppData(blob)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("negative PN cost ok with cost=%d", cost)
	}
}
