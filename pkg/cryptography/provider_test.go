package cryptography

import (
	"bytes"
	"crypto/sha256"
	"io"
	"testing"

	"golang.org/x/crypto/hkdf"
)

func TestSetProviderNilRestoresDefault(t *testing.T) {
	orig := ActiveProvider()
	SetProvider(nil)
	if _, ok := ActiveProvider().(stdlibProvider); !ok {
		t.Fatalf("expected stdlibProvider after SetProvider(nil), got %T", ActiveProvider())
	}
	SetProvider(orig)
}

func TestDeriveIdentityKeyMaterial_MatchesRFC5869HKDF(t *testing.T) {
	sharedSecret := bytes.Repeat([]byte{0xab}, 32)
	salt := bytes.Repeat([]byte{0xcd}, 16)
	context := []byte(nil)

	r := hkdf.New(sha256.New, sharedSecret, salt, context)
	want := make([]byte, IdentityKeyMaterialSize)
	if _, err := io.ReadFull(r, want); err != nil {
		t.Fatal(err)
	}

	got, err := DeriveIdentityKeyMaterial(sharedSecret, salt, context)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("DeriveIdentityKeyMaterial mismatch\nwant %x\ngot  %x", want, got)
	}
}

func TestPublicKeyFromPrivate_MatchesDeriveSharedSecretBasepoint(t *testing.T) {
	priv, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pub1, err := PublicKeyFromPrivate(priv)
	if err != nil {
		t.Fatal(err)
	}
	pub2, err := DeriveSharedSecret(priv, GetBasepoint())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pub1, pub2) {
		t.Fatalf("public key mismatch")
	}
}
