// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"encoding/hex"
	"os"
	"os/exec"
	"testing"
)

// Guarantee: mutating GetPublicKey output must not change Identity hash or
// subsequent GetPublicKey results (Python returns a fresh bytes concat).
func TestOracleGetPublicKeyMutationIsolated(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	beforeHash := append([]byte(nil), id.Hash()...)
	pub := id.GetPublicKey()
	if len(pub) != 64 {
		t.Fatalf("pubkey len=%d want 64", len(pub))
	}
	pub[0] ^= 0xFF
	pub[32] ^= 0xFF

	after := id.GetPublicKey()
	if bytes.Equal(pub, after) {
		t.Fatal("GetPublicKey returned shared mutable buffer")
	}
	if !bytes.Equal(id.Hash(), beforeHash) {
		t.Fatalf("hash mutated after caller wrote into GetPublicKey slice")
	}
	if after[0] == pub[0] || after[32] == pub[32] {
		t.Fatal("cached public key absorbed caller mutations")
	}
}

func TestOracleGetPublicKeyNilReceiver(t *testing.T) {
	var id *Identity
	if id.GetPublicKey() != nil {
		t.Fatal("nil Identity GetPublicKey must return nil")
	}
}

func TestGetPublicKeyMatchesPythonConcat(t *testing.T) {
	py := pythonInteropBin(t)
	id, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	priv, err := id.GetPrivateKey()
	if err != nil {
		t.Fatalf("GetPrivateKey: %v", err)
	}
	goPub := id.GetPublicKey()

	script := `
import sys, binascii
from RNS.Identity import Identity
raw = binascii.unhexlify(sys.argv[1])
ident = Identity(create_keys=False)
ident.load_private_key(raw)
pub = ident.get_public_key()
sys.stdout.buffer.write(pub)
`
	cmd := exec.Command(py, "-c", script, hex.EncodeToString(priv))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python get_public_key: %v\n%s", err, out)
	}
	if !bytes.Equal(out, goPub) {
		t.Fatalf("pubkey mismatch go=%x py=%x", goPub, out)
	}
	wantHash := TruncatedHash(goPub)
	if !bytes.Equal(id.Hash(), wantHash[:TruncatedHashLength/8]) {
		t.Fatalf("go hash %x want %x", id.Hash(), wantHash[:TruncatedHashLength/8])
	}
}

func pythonInteropBin(t *testing.T) string {
	t.Helper()
	candidates := []string{
		os.Getenv("PYTHON_INTEROP"),
		"/home/user1/.local/share/pipx/venvs/lxmf/bin/python",
		"python3",
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		cmd := exec.Command(c, "-c", "from RNS.Identity import Identity")
		if err := cmd.Run(); err == nil {
			return c
		}
	}
	t.Skip("python RNS not available for interop")
	return ""
}
