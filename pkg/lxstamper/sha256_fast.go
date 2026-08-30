// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package lxstamper

import (
	"encoding/binary"
	"math/bits"
)

// sha256Compress applies one SHA-256 compression round. block must be 64 bytes.
func sha256Compress(h *[8]uint32, block []byte) {
	var w [64]uint32
	for i := range 16 {
		w[i] = binary.BigEndian.Uint32(block[i*4 : i*4+4])
	}
	for i := 16; i < 64; i++ {
		v1 := w[i-2]
		s1 := bits.RotateLeft32(v1, -17) ^ bits.RotateLeft32(v1, -19) ^ (v1 >> 10)
		v0 := w[i-15]
		s0 := bits.RotateLeft32(v0, -7) ^ bits.RotateLeft32(v0, -18) ^ (v0 >> 3)
		w[i] = s1 + w[i-7] + s0 + w[i-16]
	}

	a, b, c, d, e, f, g, hh := h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7]
	for i := range 64 {
		s1 := bits.RotateLeft32(e, -6) ^ bits.RotateLeft32(e, -11) ^ bits.RotateLeft32(e, -25)
		ch := (e & f) ^ (^e & g)
		t1 := hh + s1 + ch + sha256K[i] + w[i]
		s0 := bits.RotateLeft32(a, -2) ^ bits.RotateLeft32(a, -13) ^ bits.RotateLeft32(a, -22)
		maj := (a & b) ^ (a & c) ^ (b & c)
		t2 := s0 + maj
		hh, g, f, e, d, c, b, a = g, f, e, d+t1, c, b, a, t1+t2
	}
	h[0] += a
	h[1] += b
	h[2] += c
	h[3] += d
	h[4] += e
	h[5] += f
	h[6] += g
	h[7] += hh
}

// hashMidstateStamp computes SHA256(prefix||stamp) from a midstate of prefix.
func hashMidstateStamp(ms sha256Midstate, stamp []byte) [32]byte {
	h := ms.H
	var buf [128]byte
	n := copy(buf[:], ms.Rem)
	n += copy(buf[n:], stamp)

	off := 0
	for n-off >= 64 {
		sha256Compress(&h, buf[off:off+64])
		off += 64
	}

	var block [64]byte
	remn := n - off
	copy(block[:], buf[off:off+remn])
	block[remn] = 0x80
	bitLen := uint64(ms.PrefixLen+len(stamp)) * 8
	if remn >= 56 {
		sha256Compress(&h, block[:])
		clear(block[:])
	}
	binary.BigEndian.PutUint64(block[56:], bitLen)
	sha256Compress(&h, block[:])

	var out [32]byte
	for i := range 8 {
		binary.BigEndian.PutUint32(out[i*4:], h[i])
	}
	return out
}

var sha256K = [64]uint32{
	0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
	0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
	0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
	0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
	0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
	0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
	0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
	0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
}
