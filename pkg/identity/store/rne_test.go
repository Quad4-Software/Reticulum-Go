// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package store

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func testBlob() []byte {
	b := make([]byte, 64)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

func TestRNE1RoundTrip(t *testing.T) {
	plain := testBlob()
	pass := []byte("correct horse battery staple")
	rne1, err := EncryptIdentityPayload(plain, pass)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncryptedIdentityPayload(rne1) {
		t.Fatal("encrypted payload not detected")
	}
	if len(rne1) != rneHeaderLen+len(plain)+rneTagLen {
		t.Fatalf("unexpected envelope size %d", len(rne1))
	}
	got, err := DecryptIdentityPayload(rne1, pass)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("round trip mismatch")
	}
}

func TestRNE1WrongPassphrase(t *testing.T) {
	rne1, err := EncryptIdentityPayload(testBlob(), []byte("right-pass"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecryptIdentityPayload(rne1, []byte("wrong-pass"))
	if !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("expected ErrWrongPassphrase, got %v", err)
	}
}

func TestRNE1TamperDetection(t *testing.T) {
	pass := []byte("passphrase-123")
	rne1, err := EncryptIdentityPayload(testBlob(), pass)
	if err != nil {
		t.Fatal(err)
	}
	for _, off := range []int{0, 15, 30, 54, 55, 60, len(rne1) - 1} {
		bad := append([]byte(nil), rne1...)
		bad[off] ^= 0x01
		if _, err := DecryptIdentityPayload(bad, pass); err == nil {
			t.Fatalf("tamper at offset %d not detected", off)
		}
	}
}

func TestRNE1TruncatedAndInvalid(t *testing.T) {
	pass := []byte("passphrase-123")
	rne1, err := EncryptIdentityPayload(testBlob(), pass)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, 4, 30, 54, 70} {
		if IsEncryptedIdentityPayload(rne1[:n]) {
			t.Fatalf("truncated %d-byte prefix detected as RNE1", n)
		}
	}
	// A file short only in the tag still detects as RNE1 but fails decrypt.
	if _, err := DecryptIdentityPayload(rne1[:len(rne1)-1], pass); err == nil {
		t.Fatal("truncated ciphertext decrypted")
	}
	// Bad version is not an RNE1 payload.
	bad := append([]byte(nil), rne1...)
	bad[4] = 2
	if IsEncryptedIdentityPayload(bad) {
		t.Fatal("version 2 payload detected as RNE1")
	}
	// Unknown kdf id is detected but rejected at derive time.
	bad = append([]byte(nil), rne1...)
	bad[5] = 0x7f
	if _, err := DecryptIdentityPayload(bad, pass); err == nil {
		t.Fatal("unknown kdf id accepted")
	}
	// A 64-byte plaintext identity can never collide: too short for RNE1.
	plain := testBlob()
	copy(plain, rneMagic)
	plain[4] = RNEVersion
	if IsEncryptedIdentityPayload(plain) {
		t.Fatal("64-byte plaintext misdetected as RNE1")
	}
}

func TestRNE1KDFParamBounds(t *testing.T) {
	pass := []byte("passphrase-123")
	rne1, err := EncryptIdentityPayload(testBlob(), pass)
	if err != nil {
		t.Fatal(err)
	}
	set := func(mut func(b []byte)) []byte {
		b := append([]byte(nil), rne1...)
		mut(b)
		return b
	}
	cases := map[string][]byte{
		"time zero":   set(func(b []byte) { binary.LittleEndian.PutUint32(b[6:10], 0) }),
		"time huge":   set(func(b []byte) { binary.LittleEndian.PutUint32(b[6:10], 1<<30) }),
		"memory zero": set(func(b []byte) { binary.LittleEndian.PutUint32(b[10:14], 0) }),
		"memory 4GiB": set(func(b []byte) { binary.LittleEndian.PutUint32(b[10:14], 4*1024*1024) }),
		"threads 0":   set(func(b []byte) { b[14] = 0 }),
		"threads 255": set(func(b []byte) { b[14] = 255 }),
	}
	for name, b := range cases {
		if _, err := DecryptIdentityPayload(b, pass); err == nil {
			t.Fatalf("%s: out-of-range kdf params accepted", name)
		}
	}
}

func TestRNE1Rekey(t *testing.T) {
	plain := testBlob()
	rne1, err := EncryptIdentityPayload(plain, []byte("old-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	rne2, err := RekeyIdentityPayload(rne1, []byte("old-passphrase"), []byte("new-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptIdentityPayload(rne2, []byte("new-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("rekey round trip mismatch")
	}
	if _, err := DecryptIdentityPayload(rne2, []byte("old-passphrase")); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatal("old passphrase still works after rekey")
	}
	if _, err := RekeyIdentityPayload(rne1, []byte("wrong"), []byte("x")); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatal("rekey with wrong old passphrase succeeded")
	}
}

func writePlain(t *testing.T) (string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "identity")
	blob := testBlob()
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, blob
}

func TestLoadIdentityBlobRNE1ViaEnv(t *testing.T) {
	path, plain := writePlain(t)
	pass := []byte("env-passphrase")
	rne1, err := EncryptIdentityPayload(plain, pass)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, rne1, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(PassphraseEnv, string(pass))
	got, err := LoadIdentityBlob(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("LoadIdentityBlob RNE1 mismatch")
	}
}

func TestLoadIdentityBlobRNE1NoPassphrase(t *testing.T) {
	path, plain := writePlain(t)
	rne1, err := EncryptIdentityPayload(plain, []byte("some-pass"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, rne1, 0o600); err != nil {
		t.Fatal(err)
	}
	// No env, no fd, no wrap backend, no tty in tests.
	_, err = LoadIdentityBlob(path)
	if !errors.Is(err, ErrPassphraseRequired) {
		t.Fatalf("expected ErrPassphraseRequired, got %v", err)
	}
}

func TestLoadIdentityBlobPlaintextUnchanged(t *testing.T) {
	path, plain := writePlain(t)
	got, err := LoadIdentityBlob(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("plaintext load changed")
	}
}

func TestResolvePassphrasePrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id")

	// Custom resolver beats everything.
	SetPassphraseResolver(func(string) ([]byte, error) { return []byte("resolver"), nil })
	t.Cleanup(func() { SetPassphraseResolver(nil) })
	t.Setenv(PassphraseEnv, "env-pass")
	p, err := ResolvePassphrase(path)
	if err != nil || string(p) != "resolver" {
		t.Fatalf("resolver precedence broken: %q %v", p, err)
	}
	SetPassphraseResolver(nil)

	// Env beats fd.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("fd-pass\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()
	t.Setenv(PassphraseFDEnv, fdName(r))
	p, err = ResolvePassphrase(path)
	if err != nil || string(p) != "env-pass" {
		t.Fatalf("env precedence broken: %q %v", p, err)
	}
	r.Close()

	// FD alone works and strips the trailing newline.
	os.Unsetenv(PassphraseEnv)
	r, w, err = os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("fd-pass\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()
	t.Setenv(PassphraseFDEnv, fdName(r))
	p, err = ResolvePassphrase(path)
	if err != nil || string(p) != "fd-pass" {
		t.Fatalf("fd resolution broken: %q %v", p, err)
	}

	// A configured but broken fd fails closed.
	t.Setenv(PassphraseFDEnv, "not-a-number")
	if _, err := ResolvePassphrase(path); err == nil {
		t.Fatal("broken fd env silently ignored")
	}
}

func fdName(f *os.File) string {
	return strconv.Itoa(int(f.Fd()))
}

func TestMigratePassphraseFileRoundTrip(t *testing.T) {
	path, plain := writePlain(t)
	pass := []byte("migrate-pass-1")
	if err := MigrateToPassphrase(path, pass); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncryptedIdentityPayload(raw) {
		t.Fatal("file not rewritten as RNE1")
	}
	if bytes.Contains(raw, plain[:16]) {
		t.Fatal("plaintext key material present in RNE1 file")
	}

	// Double-encrypt refused.
	if err := MigrateToPassphrase(path, pass); err == nil {
		t.Fatal("re-encrypting RNE1 file accepted")
	}

	t.Setenv(PassphraseEnv, string(pass))
	if err := MigrateEncryptedToFile(path); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, plain) {
		t.Fatal("decrypt did not restore the 64-byte plaintext")
	}

	// Rekey at file level.
	if err := MigrateToPassphrase(path, pass); err != nil {
		t.Fatal(err)
	}
	if err := RekeyEncryptedFile(path, pass, []byte("migrate-pass-2")); err != nil {
		t.Fatal(err)
	}
	t.Setenv(PassphraseEnv, "migrate-pass-2")
	got, err := LoadIdentityBlob(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("post-rekey load mismatch")
	}
}

func TestMigrateToWrappedWithMemoryBackend(t *testing.T) {
	path, plain := writePlain(t)
	mem := NewMemoryBackend()
	prev := wrapBackendCandidates
	wrapBackendCandidates = []wrapCandidate{{"mem", func() (Backend, error) { return mem, nil }}}
	t.Cleanup(func() { wrapBackendCandidates = prev })

	name, err := MigrateToWrapped(path)
	if err != nil {
		t.Fatal(err)
	}
	if name != "mem" {
		t.Fatalf("unexpected backend name %q", name)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncryptedIdentityPayload(raw) {
		t.Fatal("wrapped file is not RNE1")
	}
	// Unattended load through the wrap backend, no env needed.
	got, err := LoadIdentityBlob(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("wrapped load mismatch")
	}

	// Decrypting removes the wrap entry.
	if err := MigrateEncryptedToFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Get(AttrsForPath(path, wrapKind)); !errors.Is(err, ErrNotFound) {
		t.Fatal("wrap entry left behind after decrypt")
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, plain) {
		t.Fatal("decrypt did not restore plaintext")
	}
}

func TestMigrateToWrappedNoBackend(t *testing.T) {
	path, _ := writePlain(t)
	prev := wrapBackendCandidates
	wrapBackendCandidates = nil
	t.Cleanup(func() { wrapBackendCandidates = prev })
	if _, err := MigrateToWrapped(path); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

// A wrap passphrase and a marker-backed identity secret for the same path
// must not collide in a backend that keys on path alone.
func TestWrapAndMarkerEntriesDoNotCollide(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity")
	plain := testBlob()

	mem := NewMemoryBackend()
	prevName := BackendName()
	prev := Active()
	t.Cleanup(func() { SetActiveBackend(prevName, prev) })
	SetActiveBackend(BackendKeyring, mem)
	if err := SaveIdentityBlob(path, plain, "transport"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !IsMarkerPayload(raw) {
		t.Fatal("expected RSSI marker")
	}
	// Keep mem as the active marker backend so marker resolution hits it
	// rather than probing the real keyring or Secret Service.

	prevC := wrapBackendCandidates
	wrapBackendCandidates = []wrapCandidate{{"mem", func() (Backend, error) { return mem, nil }}}
	t.Cleanup(func() { wrapBackendCandidates = prevC })

	if _, err := MigrateToWrapped(path); err != nil {
		t.Fatal(err)
	}
	// The wrap secret must still resolve after the marker entry was dropped.
	got, err := LoadIdentityBlob(path)
	if err != nil {
		t.Fatalf("wrapped identity unloadable after marker cleanup: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("identity changed through marker to wrapped migration")
	}
}

func TestMigrateToPassphraseRejectsNonIdentity(t *testing.T) {
	dir := t.TempDir()
	for name, blob := range map[string][]byte{
		"rhb1-72":   append(append([]byte("RHB1"), 1, 0, 0, 0), make([]byte, 64)...),
		"garbage":   bytes.Repeat([]byte{0xab}, 100),
		"too-short": bytes.Repeat([]byte{0x01}, 32),
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, blob, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := MigrateToPassphrase(path, []byte("some-pass-123")); err == nil {
			t.Fatalf("%s: non-64-byte input encrypted", name)
		}
		// File must be untouched on rejection.
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw, blob) {
			t.Fatalf("%s: file modified on rejected migration", name)
		}
	}
}

func TestRNE1OutputPermissions(t *testing.T) {
	path, _ := writePlain(t)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MigrateToPassphrase(path, []byte("some-pass-123")); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("RNE1 file mode %o, expected 0600", fi.Mode().Perm())
	}
}

func TestMigrateEncryptedToFileRejectsShortPayload(t *testing.T) {
	// Hand-craft an RNE1 envelope around a non-identity payload.
	dir := t.TempDir()
	path := filepath.Join(dir, "identity")
	pass := []byte("some-pass-123")
	rne1, err := EncryptIdentityPayload([]byte("not-an-identity"), pass)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, rne1, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(PassphraseEnv, string(pass))
	if err := MigrateEncryptedToFile(path); err == nil {
		t.Fatal("short decrypted payload written as identity file")
	}
	// Still RNE1, not a corrupted plaintext file.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncryptedIdentityPayload(raw) {
		t.Fatal("file corrupted by rejected decrypt")
	}
}

func TestReadPassphraseFDStrictParsing(t *testing.T) {
	for _, bad := range []string{"3abc", "-1", "", " 3", "0x3"} {
		if _, err := readPassphraseFD(bad); err == nil {
			t.Fatalf("fd %q accepted", bad)
		}
	}
}
