// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package buffer

const (
	StreamIDMax   = 0x3fff
	MaxChunkLen   = 16 * 1024
	MaxDataLen    = 457
	CompressTries = 4

	StreamHeaderEOF        = 0x8000
	StreamHeaderCompressed = 0x4000

	StreamDataMessageType = 0x01

	StreamHeaderSize = 2

	CompressThreshold = 32
)
