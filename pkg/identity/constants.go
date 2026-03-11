// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package identity

const (
	CURVE                = "Curve25519"
	KEYSIZE              = 512
	RATCHETSIZE          = 256
	RATCHET_EXPIRY       = 2592000
	TRUNCATED_HASHLENGTH = 128
	NAME_HASH_LENGTH     = 80

	TOKEN_OVERHEAD   = 16
	AES128_BLOCKSIZE = 16
	HASHLENGTH       = 256
	SIGLENGTH        = KEYSIZE

	RATCHET_ROTATION_INTERVAL = 1800
	MAX_RETAINED_RATCHETS     = 512
)
