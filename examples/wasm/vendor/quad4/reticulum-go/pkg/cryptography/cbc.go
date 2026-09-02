// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"crypto/cipher"
	"crypto/subtle"
	"errors"
)

var errCBCArgs = errors.New("invalid CBC arguments")

// EncryptCBC encrypts buf in place with AES-CBC. iv is the initialization
// vector and is not written to buf. buf must be a multiple of the block size.
func EncryptCBC(block cipher.Block, iv, buf []byte) error {
	if block == nil {
		return errCBCArgs
	}
	bs := block.BlockSize()
	if len(iv) != bs || len(buf)%bs != 0 {
		return errCBCArgs
	}
	prev := iv
	for i := 0; i < len(buf); i += bs {
		chunk := buf[i : i+bs]
		subtle.XORBytes(chunk, chunk, prev)
		block.Encrypt(chunk, chunk)
		prev = chunk
	}
	return nil
}

// DecryptCBC decrypts src into dst with AES-CBC. src and dst may be the same
// slice. iv is the initialization vector. src and dst must be the same length
// and a multiple of the block size.
func DecryptCBC(block cipher.Block, iv, src, dst []byte) error {
	if block == nil {
		return errCBCArgs
	}
	bs := block.BlockSize()
	if len(iv) != bs || len(src) != len(dst) || len(src)%bs != 0 {
		return errCBCArgs
	}
	var prev, ciph [32]byte
	if bs > len(prev) {
		return errCBCArgs
	}
	copy(prev[:], iv)
	for i := 0; i < len(src); i += bs {
		copy(ciph[:], src[i:i+bs])
		block.Decrypt(dst[i:i+bs], src[i:i+bs])
		subtle.XORBytes(dst[i:i+bs], dst[i:i+bs], prev[:bs])
		copy(prev[:], ciph[:bs])
	}
	return nil
}
