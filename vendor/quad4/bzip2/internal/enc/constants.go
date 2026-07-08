// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

package enc

const (
	BZMaxAlphaSize = 258
	BZMaxCodeLen   = 23
	BZNGroups      = 6
	BZGSize        = 50
	BZNIters       = 4
	BZMaxSelectors = 2 + (900000 / BZGSize)

	BZRunA = 0
	BZRunB = 1

	BZLesserICost  = 0
	BZGreaterICost = 15
	BZMaxHuffLen   = 17
	BZNOvershoot   = 32
	BZFileMagic    = 0x425a
	BZBlockMagicHi = 0x314159265359
	BZFinalMagicHi = 0x177245385090
)
