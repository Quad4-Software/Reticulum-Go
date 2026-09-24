// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package store

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/Quad4-Software/Reticulum-Go/pkg/securemem"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// RNE1 is the passphrase-encrypted identity file format.
//
// Layout (135 bytes for a 64-byte identity payload):
//
//	offset 0:  magic "RNE1"            (4)
//	offset 4:  version uint8 = 1       (1)
//	offset 5:  kdf_id uint8            (1)  0x01 = argon2id
//	offset 6:  kdf time uint32le       (4)
//	offset 10: kdf memory KiB uint32le (4)
//	offset 14: kdf threads uint8       (1)
//	offset 15: salt                    (16)
//	offset 31: XChaCha20 nonce         (24)
//	offset 55: ciphertext              (payload length)
//	offset 55+n: poly1305 tag          (16)
//
// The ciphertext covers exactly the plaintext identity blob. For software
// identities that is the standard 64-byte payload, so an RNE1 file decrypts
// to the same bytes Python RNS and the plaintext file format use.
const (
	rneMagicLen  = 4
	rneHeaderLen = 55 // magic + version + kdf_id + params + salt + nonce
	rneTagLen    = 16
	rneSaltLen   = 16
	rneNonceLen  = chacha20poly1305.NonceSizeX

	rneOffVersion = 4
	rneOffKDFID   = 5
	rneOffTime    = 6
	rneOffMemory  = 10
	rneOffThreads = 14
	rneOffSalt    = 15
	rneOffNonce   = 31

	RNEVersion  = 1
	KDFArgon2id = 0x01

	// RFC 9106 second recommended parameter set: m=64 MiB, t=3, p=4.
	argonDefaultTime    uint32 = 3
	argonDefaultMemory  uint32 = 64 * 1024
	argonDefaultThreads uint8  = 4
	argonKeyLen         uint32 = 32

	// Bounds on KDF parameters accepted from files. Files we write always
	// carry the defaults above; the caps stop a crafted header from
	// requesting unbounded work or memory at load time.
	argonMaxTime    uint32 = 64
	argonMaxMemory  uint32 = 2 * 1024 * 1024
	argonMaxThreads uint8  = 64
)

var rneMagic = []byte("RNE1")

var (
	ErrPassphraseRequired = errors.New("identity store: passphrase required")
	ErrWrongPassphrase    = errors.New("identity store: passphrase incorrect or file corrupted")
)

// IsEncryptedIdentityPayload reports whether data is an RNE1 v1 envelope.
func IsEncryptedIdentityPayload(data []byte) bool {
	if len(data) < rneHeaderLen+rneTagLen {
		return false
	}
	if !bytes.Equal(data[:rneMagicLen], rneMagic) {
		return false
	}
	return data[rneOffVersion] == RNEVersion
}

type kdfParams struct {
	id      uint8
	time    uint32
	memory  uint32
	threads uint8
}

func defaultKDFParams() kdfParams {
	return kdfParams{id: KDFArgon2id, time: argonDefaultTime, memory: argonDefaultMemory, threads: argonDefaultThreads}
}

func (p kdfParams) derive(passphrase, salt []byte) ([]byte, error) {
	switch p.id {
	case KDFArgon2id:
		if p.time == 0 || p.time > argonMaxTime ||
			p.memory == 0 || p.memory > argonMaxMemory ||
			p.threads == 0 || p.threads > argonMaxThreads {
			return nil, errors.New("identity store: argon2 parameters out of range")
		}
		return argon2.IDKey(passphrase, salt, p.time, p.memory, p.threads, argonKeyLen), nil
	default:
		return nil, fmt.Errorf("identity store: unsupported kdf id 0x%02x", p.id)
	}
}

// EncryptIdentityPayload wraps a plaintext identity blob in an RNE1 envelope.
func EncryptIdentityPayload(plaintext, passphrase []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errors.New("identity store: empty identity payload")
	}
	if len(passphrase) == 0 {
		return nil, ErrPassphraseRequired
	}
	return encryptWithParams(plaintext, passphrase, defaultKDFParams())
}

func encryptWithParams(plaintext, passphrase []byte, p kdfParams) ([]byte, error) {
	salt := make([]byte, rneSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	nonce := make([]byte, rneNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	key, err := p.derive(passphrase, salt)
	if err != nil {
		return nil, err
	}
	defer securemem.WipeBytes(key)

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, rneHeaderLen+len(plaintext)+rneTagLen)
	out = append(out, rneMagic...)
	out = append(out, RNEVersion, p.id)
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], p.time)
	out = append(out, u32[:]...)
	binary.LittleEndian.PutUint32(u32[:], p.memory)
	out = append(out, u32[:]...)
	out = append(out, p.threads)
	out = append(out, salt...)
	out = append(out, nonce...)
	out = aead.Seal(out, nonce, plaintext, out[:rneHeaderLen])
	return out, nil
}

// DecryptIdentityPayload unwraps an RNE1 envelope and returns the plaintext blob.
func DecryptIdentityPayload(rne1, passphrase []byte) ([]byte, error) {
	if !IsEncryptedIdentityPayload(rne1) {
		return nil, errors.New("identity store: not an RNE1 payload")
	}
	if len(passphrase) == 0 {
		return nil, ErrPassphraseRequired
	}
	p := kdfParams{
		id:      rne1[rneOffKDFID],
		time:    binary.LittleEndian.Uint32(rne1[rneOffTime : rneOffTime+4]),
		memory:  binary.LittleEndian.Uint32(rne1[rneOffMemory : rneOffMemory+4]),
		threads: rne1[rneOffThreads],
	}
	salt := rne1[rneOffSalt:rneOffNonce]
	nonce := rne1[rneOffNonce:rneHeaderLen]
	sealed := rne1[rneHeaderLen:]
	if len(sealed) < rneTagLen {
		return nil, errors.New("identity store: truncated RNE1 ciphertext")
	}

	key, err := p.derive(passphrase, salt)
	if err != nil {
		return nil, err
	}
	defer securemem.WipeBytes(key)

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	plain, err := aead.Open(nil, nonce, sealed, rne1[:rneHeaderLen])
	if err != nil {
		return nil, ErrWrongPassphrase
	}
	return plain, nil
}

// RekeyIdentityPayload re-encrypts an RNE1 envelope under a new passphrase.
func RekeyIdentityPayload(rne1, oldPass, newPass []byte) ([]byte, error) {
	plain, err := DecryptIdentityPayload(rne1, oldPass)
	if err != nil {
		return nil, err
	}
	defer securemem.WipeBytes(plain)
	return EncryptIdentityPayload(plain, newPass)
}
