// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package cryptography

import "crypto/ed25519"

type stdlibProvider struct{}

func (stdlibProvider) GenerateKeyPair() (privateKey, publicKey []byte, err error) {
	return implGenerateKeyPair()
}

func (stdlibProvider) PublicKeyFromPrivate(privateKey []byte) ([]byte, error) {
	return implPublicKeyFromPrivate(privateKey)
}

func (stdlibProvider) DeriveSharedSecret(privateKey, peerPublicKey []byte) ([]byte, error) {
	return implDeriveSharedSecret(privateKey, peerPublicKey)
}

func (stdlibProvider) GetBasepoint() []byte {
	return implGetBasepoint()
}

func (stdlibProvider) GenerateSigningKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return implGenerateSigningKeyPair()
}

func (stdlibProvider) Sign(privateKey ed25519.PrivateKey, message []byte) []byte {
	return implSign(privateKey, message)
}

func (stdlibProvider) Verify(publicKey ed25519.PublicKey, message, signature []byte) bool {
	return implVerify(publicKey, message, signature)
}

func (stdlibProvider) EncryptAES256CBC(key, plaintext []byte) ([]byte, error) {
	return implEncryptAES256CBC(key, plaintext)
}

func (stdlibProvider) DecryptAES256CBC(key, ciphertext []byte) ([]byte, error) {
	return implDecryptAES256CBC(key, ciphertext)
}

func (stdlibProvider) ComputeHMAC(key, message []byte) []byte {
	return implComputeHMAC(key, message)
}

func (stdlibProvider) ValidateHMAC(key, message, messageHMAC []byte) bool {
	return implValidateHMAC(key, message, messageHMAC)
}

func (stdlibProvider) Hash(data []byte) []byte {
	return implHash(data)
}

func (stdlibProvider) DeriveKey(secret, salt, info []byte, length int) ([]byte, error) {
	return implDeriveKey(secret, salt, info, length)
}

func (stdlibProvider) ExpandEncryptWithHMACKeyMaterial(key32 []byte) (hmacKey, aesKey []byte, err error) {
	return implExpandEncryptWithHMACKeyMaterial(key32)
}

func (stdlibProvider) DeriveIdentityKeyMaterial(sharedSecret, salt, context []byte) ([]byte, error) {
	return implDeriveIdentityKeyMaterial(sharedSecret, salt, context)
}
