// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package identity

const (
	Curve               = "Curve25519"
	KeySize             = 512
	RatchetSize         = 256
	RatchetExpiry       = 2592000
	TruncatedHashLength = 128
	NameHashLength      = 80

	TokenOverhead   = 16
	AES128BlockSize = 16
	HashLength      = 256
	SigLength       = KeySize

	RatchetRotationInterval = 1800
	MaxRetainedRatchets     = 512
)
