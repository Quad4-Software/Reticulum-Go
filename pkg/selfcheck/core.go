// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package selfcheck

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

func checkRuntime() []Result {
	out := make([]Result, 0, 3)
	out = append(out, result("runtime/platform", SeverityPass,
		fmt.Sprintf("%s/%s %s", runtime.GOOS, runtime.GOARCH, goVersion())))

	cgo := "disabled"
	if os.Getenv("CGO_ENABLED") == "1" {
		cgo = "enabled"
	} else if os.Getenv("CGO_ENABLED") == "" {
		cgo = "default"
	}
	out = append(out, result("runtime/cgo", SeverityPass, cgo))

	dir, err := os.MkdirTemp("", "rns-selfcheck-runtime-*")
	if err != nil {
		out = append(out, result("runtime/tempdir", SeverityFail, err.Error()))
		return out
	}
	defer os.RemoveAll(dir)
	probe := filepath.Join(dir, "probe")
	if err := os.WriteFile(probe, []byte("ok"), fileModePrivate); err != nil {
		out = append(out, result("runtime/tempdir", SeverityFail, err.Error()))
		return out
	}
	out = append(out, result("runtime/tempdir", SeverityPass, dir))
	return out
}

func checkCrypto() []Result {
	out := make([]Result, 0, 3)

	aPriv, aPub, err := cryptography.GenerateKeyPair()
	if err != nil {
		out = append(out, result("crypto/x25519", SeverityFail, err.Error()))
	} else {
		bPriv, bPub, err := cryptography.GenerateKeyPair()
		if err != nil {
			out = append(out, result("crypto/x25519", SeverityFail, err.Error()))
		} else {
			s1, err1 := cryptography.DeriveSharedSecret(aPriv, bPub)
			s2, err2 := cryptography.DeriveSharedSecret(bPriv, aPub)
			if err1 != nil || err2 != nil {
				out = append(out, result("crypto/x25519", SeverityFail, fmt.Sprintf("%v %v", err1, err2)))
			} else if !bytes.Equal(s1, s2) {
				out = append(out, result("crypto/x25519", SeverityFail, "shared secrets differ"))
			} else {
				out = append(out, result("crypto/x25519", SeverityPass, "shared secret match"))
			}
		}
	}

	pub, priv, err := cryptography.GenerateSigningKeyPair()
	if err != nil {
		out = append(out, result("crypto/ed25519", SeverityFail, err.Error()))
	} else {
		msg := []byte("reticulum-go self-check")
		sig := cryptography.Sign(priv, msg)
		if !cryptography.Verify(pub, msg, sig) {
			out = append(out, result("crypto/ed25519", SeverityFail, "verify failed"))
		} else {
			out = append(out, result("crypto/ed25519", SeverityPass, "sign and verify"))
		}
	}

	key, err := cryptography.GenerateAES256Key()
	if err != nil {
		out = append(out, result("crypto/aes256-cbc", SeverityFail, err.Error()))
		return out
	}
	plain := []byte("self-check plaintext")
	ct, err := cryptography.EncryptAES256CBC(key, plain)
	if err != nil {
		out = append(out, result("crypto/aes256-cbc", SeverityFail, err.Error()))
		return out
	}
	pt, err := cryptography.DecryptAES256CBC(key, ct)
	if err != nil {
		out = append(out, result("crypto/aes256-cbc", SeverityFail, err.Error()))
		return out
	}
	if !bytes.Equal(pt, plain) {
		out = append(out, result("crypto/aes256-cbc", SeverityFail, "plaintext mismatch"))
		return out
	}
	out = append(out, result("crypto/aes256-cbc", SeverityPass, "encrypt and decrypt"))
	return out
}

func checkIdentityFile(opts Options) []Result {
	parent := opts.WorkDir
	if parent == "" {
		parent = os.TempDir()
	}
	dir, err := os.MkdirTemp(parent, "rns-selfcheck-id-*")
	if err != nil {
		return []Result{result(nameIdentityFile, SeverityFail, err.Error())}
	}
	defer os.RemoveAll(dir)

	id, err := identity.New()
	if err != nil {
		return []Result{result(nameIdentityFile, SeverityFail, err.Error())}
	}
	defer id.Close()

	path := filepath.Join(dir, "identity")
	if err := id.ToFile(path); err != nil {
		return []Result{result(nameIdentityFile, SeverityFail, "save: "+err.Error())}
	}
	loaded, err := identity.FromFile(path)
	if err != nil {
		return []Result{result(nameIdentityFile, SeverityFail, "load: "+err.Error())}
	}
	defer loaded.Close()
	if id.GetHexHash() != loaded.GetHexHash() {
		return []Result{result(nameIdentityFile, SeverityFail, "hash mismatch after reload")}
	}
	return []Result{result(nameIdentityFile, SeverityPass, id.GetHexHash())}
}

func checkPacket() Result {
	p := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		DestinationHash: make([]byte, packet.TruncatedHashLength),
		Data:            []byte("self-check"),
	}
	if err := p.Pack(); err != nil {
		return result("packet/roundtrip", SeverityFail, "pack: "+err.Error())
	}
	raw := append([]byte(nil), p.Raw...)
	p2 := &packet.Packet{Raw: raw}
	if err := p2.Unpack(); err != nil {
		return result("packet/roundtrip", SeverityFail, "unpack: "+err.Error())
	}
	if !bytes.Equal(p2.Data, p.Data) {
		return result("packet/roundtrip", SeverityFail, "data mismatch")
	}
	return result("packet/roundtrip", SeverityPass, fmt.Sprintf("%d bytes", len(raw)))
}

func checkBinaryCLI(opts Options) Result {
	path := opts.BinaryPath
	if path == "" {
		return result("binary/cli", SeveritySkip, "BinaryPath not set")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return result("binary/cli", SeverityFail, err.Error())
	}
	path = abs
	if _, err := os.Stat(path); err != nil {
		return result("binary/cli", SeverityFail, err.Error())
	}
	cmd := exec.Command(path, "--version") // #nosec G204 -- BinaryPath from CLI or CI wrapper
	out, err := cmd.CombinedOutput()
	if err != nil {
		return result("binary/cli", SeverityFail, fmt.Sprintf("%v: %s", err, bytes.TrimSpace(out)))
	}
	return result("binary/cli", SeverityPass, string(bytes.TrimSpace(out)))
}
