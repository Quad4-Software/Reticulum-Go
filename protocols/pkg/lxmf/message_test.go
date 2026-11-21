// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

const (
	testTitle   = "subject"
	testContent = "Hello, LXMF!"
)

type fixedSigner struct{ sig []byte }

func (f *fixedSigner) Sign(_ []byte) ([]byte, error) { return f.sig, nil }

type errSigner struct{}

func (errSigner) Sign(_ []byte) ([]byte, error) { return nil, errors.New("nope") }

func mustNewIdentity(tb testing.TB) *identity.Identity {
	tb.Helper()
	id, err := identity.NewIdentity()
	if err != nil {
		tb.Fatalf("identity.NewIdentity: %v", err)
	}
	return id
}

func TestNewMessage_ValidatesHashLengths(t *testing.T) {
	if _, err := NewMessage(make([]byte, 8), make([]byte, 16), nil, nil, nil); !errors.Is(err, ErrInvalidHashLength) {
		t.Fatalf("expected ErrInvalidHashLength for short destination, got %v", err)
	}
	if _, err := NewMessage(make([]byte, 16), make([]byte, 8), nil, nil, nil); !errors.Is(err, ErrInvalidHashLength) {
		t.Fatalf("expected ErrInvalidHashLength for short source, got %v", err)
	}
}

func TestPack_ProducesExpectedLayout(t *testing.T) {
	dst := make([]byte, DestinationLength)
	src := make([]byte, DestinationLength)
	for i := range dst {
		dst[i] = byte(i)
		src[i] = byte(0xff - i)
	}

	msg, err := NewMessage(dst, src, []byte(testTitle), []byte(testContent), nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	msg.Timestamp = 1700000000.5

	signer := &fixedSigner{sig: bytes.Repeat([]byte{0xAB}, SignatureLength)}
	packed, err := msg.Pack(signer)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	if len(packed) <= 2*DestinationLength+SignatureLength {
		t.Fatalf("packed too short: %d", len(packed))
	}
	if !bytes.Equal(packed[:DestinationLength], dst) {
		t.Errorf("destination prefix mismatch")
	}
	if !bytes.Equal(packed[DestinationLength:2*DestinationLength], src) {
		t.Errorf("source prefix mismatch")
	}
	if !bytes.Equal(packed[2*DestinationLength:2*DestinationLength+SignatureLength], signer.sig) {
		t.Errorf("signature mismatch")
	}

	hashedPayload, err := encodePayload(msg.payloadList(false))
	if err != nil {
		t.Fatalf("encodePayload: %v", err)
	}
	expected := sha256.Sum256(append(append(append([]byte{}, dst...), src...), hashedPayload...))
	if !bytes.Equal(msg.Hash, expected[:]) {
		t.Errorf("hash mismatch")
	}
}

func TestPack_RequiresSigner(t *testing.T) {
	dst := make([]byte, DestinationLength)
	src := make([]byte, DestinationLength)
	msg, err := NewMessage(dst, src, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if _, err := msg.Pack(nil); err == nil {
		t.Fatal("expected error for nil signer")
	}
	if _, err := msg.Pack(errSigner{}); err == nil {
		t.Fatal("expected propagated signer error")
	}
}

func TestPackUnpack_RoundTrip(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)

	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)
	identity.Remember(nil, dst.Hash(), dst.GetPublicKey(), nil)

	fields := map[byte]any{
		FieldRenderer: []byte{RendererMarkdown},
	}
	msg, err := NewMessage(dst.Hash(), src.Hash(), []byte(testTitle), []byte(testContent), fields)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	packed, err := msg.Pack(src)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	got, err := Unpack(packed, RecallSource)
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	if !got.SignatureValidated {
		t.Fatalf("signature not validated, reason=%d", got.UnverifiedReason)
	}
	if got.TitleString() != testTitle {
		t.Errorf("title mismatch: %q", got.TitleString())
	}
	if got.ContentString() != testContent {
		t.Errorf("content mismatch: %q", got.ContentString())
	}
	if got.Fields == nil {
		t.Fatalf("fields lost during round-trip")
	}
	rv, ok := got.Fields[FieldRenderer].([]byte)
	if !ok || len(rv) == 0 || rv[0] != RendererMarkdown {
		t.Errorf("renderer field round-trip failed: %#v", got.Fields[FieldRenderer])
	}
	if !bytes.Equal(got.Hash, msg.Hash) {
		t.Errorf("hash mismatch: %s vs %s", hex.EncodeToString(got.Hash), hex.EncodeToString(msg.Hash))
	}
}

func TestUnpack_DetectsTamperedSignature(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)

	identity.Remember(nil, src.Hash(), src.GetPublicKey(), nil)

	msg, err := NewMessage(dst.Hash(), src.Hash(), []byte(testTitle), []byte("untouched"), nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	packed, err := msg.Pack(src)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	tampered := append([]byte(nil), packed...)
	tampered[2*DestinationLength+10] ^= 0x01

	_, err = Unpack(tampered, RecallSource)
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("expected ErrSignatureInvalid, got %v", err)
	}
}

func TestUnpack_UnknownSourceMarksUnverified(t *testing.T) {
	src := mustNewIdentity(t)
	dst := mustNewIdentity(t)

	msg, err := NewMessage(dst.Hash(), src.Hash(), nil, []byte("ping"), nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	packed, err := msg.Pack(src)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	got, err := Unpack(packed, func(_ []byte) (*identity.Identity, error) { return nil, nil })
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	if got.SignatureValidated {
		t.Fatal("expected signature to be unvalidated")
	}
	if got.UnverifiedReason != UnverifiedSourceUnknown {
		t.Fatalf("expected UnverifiedSourceUnknown, got %d", got.UnverifiedReason)
	}
}

func TestUnpack_TooShort(t *testing.T) {
	if _, err := Unpack(make([]byte, 10), RecallSource); !errors.Is(err, ErrMessageTooShort) {
		t.Fatalf("expected ErrMessageTooShort, got %v", err)
	}
}
