// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

package enc

var crctab [256]uint32

func init() {
	const poly = 0x04C11DB7
	for i := range crctab {
		crc := uint32(i) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ poly
			} else {
				crc <<= 1
			}
		}
		crctab[i] = crc
	}
}

func bzUpdateCRC(crc uint32, b byte) uint32 {
	return (crc << 8) ^ crctab[byte(crc>>24)^b]
}

func blockCRCFromBytes(data []byte) uint32 {
	crc := uint32(0xffffffff)
	for _, v := range data {
		crc = bzUpdateCRC(crc, v)
	}
	return ^crc
}
