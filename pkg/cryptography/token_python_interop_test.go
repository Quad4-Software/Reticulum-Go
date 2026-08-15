// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"bytes"
	"encoding/hex"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const rnsTokenScript = `
from RNS.Cryptography.Token import Token
from RNS.Cryptography.AES import AES_256_CBC
from RNS.Cryptography.PKCS7 import PKCS7
import sys
mode, key_hex, data_hex = sys.argv[1], sys.argv[2], sys.argv[3]
key = bytes.fromhex(key_hex)
data = bytes.fromhex(data_hex) if data_hex else b""
if mode == "token_encrypt":
    sys.stdout.write(Token(key).encrypt(data).hex())
elif mode == "token_decrypt":
    sys.stdout.write(Token(key).decrypt(data).hex())
elif mode == "aes_decrypt":
    iv, body = data[:16], data[16:]
    pt = PKCS7.unpad(AES_256_CBC.decrypt(body, key, iv))
    sys.stdout.write(pt.hex())
else:
    raise SystemExit("bad mode")
`

func rnsPython(t *testing.T) string {
	t.Helper()
	exe := os.Getenv("PYTHON_INTEROP")
	if exe == "" {
		exe = "python3"
	}
	cmd := exec.Command(exe, "-c", "from RNS.Cryptography.Token import Token")
	if err := cmd.Run(); err != nil {
		if os.Getenv("RUN_PY_INTEROP") != "" {
			t.Fatalf("python RNS Token required: %v", err)
		}
		t.Skip("python RNS not available")
	}
	return exe
}

func rnsRun(t *testing.T, exe, mode string, key, data []byte) []byte {
	t.Helper()
	cmd := exec.Command(exe, "-c", rnsTokenScript, mode, hex.EncodeToString(key), hex.EncodeToString(data))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python %s: %v\n%s", mode, err, out)
	}
	got := strings.TrimSpace(string(out))
	if got == "" {
		return nil
	}
	b, err := hex.DecodeString(got)
	if err != nil {
		t.Fatalf("python %s hex: %v (%q)", mode, err, got)
	}
	return b
}

func TestPythonRNSTokenRoundTrip(t *testing.T) {
	exe := rnsPython(t)
	plaintexts := [][]byte{
		nil,
		[]byte("A"),
		[]byte("hello reticulum protocol"),
		bytes.Repeat([]byte{'x'}, 16),
		bytes.Repeat([]byte{'x'}, 17),
	}
	for _, size := range []int{TokenKeySize, TokenKeySize128} {
		key := bytesSeq(size)
		for _, pt := range plaintexts {
			goTok, err := EncryptToken(key, pt)
			if err != nil {
				t.Fatal(err)
			}
			fromPy := rnsRun(t, exe, "token_decrypt", key, goTok)
			if !bytes.Equal(fromPy, pt) {
				t.Fatalf("python decrypt of Go token size=%d pt=%q got %q", size, pt, fromPy)
			}

			pyTok := rnsRun(t, exe, "token_encrypt", key, pt)
			fromGo, err := DecryptToken(key, pyTok)
			if err != nil {
				t.Fatalf("Go decrypt of python token size=%d: %v", size, err)
			}
			if !bytes.Equal(fromGo, pt) {
				t.Fatalf("Go decrypt of python token size=%d got %q want %q", size, fromGo, pt)
			}
		}
	}
	t.Log("PROVED TOKEN_PYTHON_RNS_LIVE")
}

func TestPythonRNSAES256CBCDecryptsGo(t *testing.T) {
	exe := rnsPython(t)
	key := bytesSeq(32)
	pt := []byte("hello reticulum protocol")
	ct, err := EncryptAES256CBC(key, pt)
	if err != nil {
		t.Fatal(err)
	}
	got := rnsRun(t, exe, "aes_decrypt", key, ct)
	if !bytes.Equal(got, pt) {
		t.Fatalf("python AES decrypt got %q want %q", got, pt)
	}
	t.Log("PROVED AES256_PYTHON_RNS_LIVE")
}
