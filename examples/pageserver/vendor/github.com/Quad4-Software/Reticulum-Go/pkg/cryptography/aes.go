// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"crypto/aes"
	"crypto/rand"
	"errors"
	"io"
)

func GenerateAES256Key() ([]byte, error) {
	key := make([]byte, AES256KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

func implEncryptAES256CBC(key, plaintext []byte) ([]byte, error) {
	if len(key) != AES256KeySize {
		return nil, errors.New("invalid key size: must be 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	ctLen := len(plaintext) + padding
	out := make([]byte, aes.BlockSize+ctLen)
	if _, err := io.ReadFull(rand.Reader, out[:aes.BlockSize]); err != nil {
		return nil, err
	}
	copy(out[aes.BlockSize:aes.BlockSize+len(plaintext)], plaintext)
	padByte := byte(padding)
	for i := aes.BlockSize + len(plaintext); i < aes.BlockSize+ctLen; i++ {
		out[i] = padByte
	}
	if err := EncryptCBC(block, out[:aes.BlockSize], out[aes.BlockSize:]); err != nil {
		return nil, err
	}
	return out, nil
}

func implDecryptAES256CBC(key, ciphertext []byte) ([]byte, error) {
	if len(key) != AES256KeySize {
		return nil, errors.New("invalid key size: must be 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < aes.BlockSize {
		return nil, errors.New("ciphertext is too short")
	}

	iv := ciphertext[:aes.BlockSize]
	ct := ciphertext[aes.BlockSize:]
	if len(ct)%aes.BlockSize != 0 {
		return nil, errors.New("ciphertext is not a multiple of the block size")
	}
	plaintext := make([]byte, len(ct))
	if err := DecryptCBC(block, iv, ct, plaintext); err != nil {
		return nil, err
	}
	return RemovePKCS7Padding(plaintext)
}

// EncryptAES256CBC encrypts data using AES-256 in CBC mode.
// The IV is prepended to the ciphertext.
func EncryptAES256CBC(key, plaintext []byte) ([]byte, error) {
	return ActiveProvider().EncryptAES256CBC(key, plaintext)
}

// DecryptAES256CBC decrypts data using AES-256 in CBC mode.
// It assumes the IV is prepended to the ciphertext.
func DecryptAES256CBC(key, ciphertext []byte) ([]byte, error) {
	return ActiveProvider().DecryptAES256CBC(key, ciphertext)
}
