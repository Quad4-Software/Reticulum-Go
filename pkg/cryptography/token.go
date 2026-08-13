// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"crypto/aes"
	"crypto/cipher"
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
	out := make([]byte, aes.BlockSize+len(padtext))
	copy(out[:aes.BlockSize], iv)
	mode.CryptBlocks(out[aes.BlockSize:], padtext)
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
	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ct))
	mode.CryptBlocks(plaintext, ct)
	return RemovePKCS7Padding(plaintext)
}
