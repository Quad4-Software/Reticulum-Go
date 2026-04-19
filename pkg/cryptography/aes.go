// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package cryptography

import (
	"crypto/aes"
	"crypto/cipher"
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

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	padtext := make([]byte, len(plaintext)+padding)
	copy(padtext, plaintext)
	for i := len(plaintext); i < len(padtext); i++ {
		padtext[i] = byte(padding)
	}

	mode := cipher.NewCBCEncrypter(block, iv) // #nosec G407
	ciphertext := make([]byte, len(padtext))
	mode.CryptBlocks(ciphertext, padtext)

	return append(iv, ciphertext...), nil
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
	ciphertext = ciphertext[aes.BlockSize:]

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("ciphertext is not a multiple of the block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

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
