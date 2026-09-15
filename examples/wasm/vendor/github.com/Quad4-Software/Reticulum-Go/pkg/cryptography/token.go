// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"crypto/aes"
	"crypto/rand"
	"errors"
	"io"
)

// GenerateTokenKey returns a 64-byte AES-256 Token key (Python Token.generate_key).
func GenerateTokenKey() ([]byte, error) {
	key := make([]byte, TokenKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

func splitTokenKey(key []byte) (hmacKey, encKey []byte, err error) {
	switch len(key) {
	case TokenKeySize128:
		return key[:16], key[16:], nil
	case TokenKeySize:
		return key[:32], key[32:], nil
	default:
		return nil, nil, errors.New("token key must be 32 or 64 bytes")
	}
}

// EncryptToken builds a Python Token: IV || AES-CBC ciphertext || HMAC-SHA256.
func EncryptToken(key, plaintext []byte) ([]byte, error) {
	hmacKey, encKey, err := splitTokenKey(key)
	if err != nil {
		return nil, err
	}
	ct, err := encryptAESCBC(encKey, plaintext)
	if err != nil {
		return nil, err
	}
	mac := ComputeHMAC(hmacKey, ct)
	return append(ct, mac...), nil
}

// DecryptToken verifies HMAC then AES-CBC decrypts a Python Token.
func DecryptToken(key, token []byte) ([]byte, error) {
	hmacKey, encKey, err := splitTokenKey(key)
	if err != nil {
		return nil, err
	}
	if len(token) < aes.BlockSize+SHA256Size {
		return nil, errors.New("token too short")
	}
	mac := token[len(token)-SHA256Size:]
	body := token[:len(token)-SHA256Size]
	if !ValidateHMAC(hmacKey, body, mac) {
		return nil, errors.New("token HMAC was invalid")
	}
	return decryptAESCBC(encKey, body)
}

func encryptAESCBC(key, plaintext []byte) ([]byte, error) {
	if len(key) == AES256KeySize {
		return EncryptAES256CBC(key, plaintext)
	}
	if len(key) != 16 {
		return nil, errors.New("invalid AES key size")
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

func decryptAESCBC(key, ciphertext []byte) ([]byte, error) {
	if len(key) == AES256KeySize {
		return DecryptAES256CBC(key, ciphertext)
	}
	if len(key) != 16 {
		return nil, errors.New("invalid AES key size")
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
