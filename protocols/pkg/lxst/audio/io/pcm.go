// SPDX-License-Identifier: LicenseRef-Reticulum
//
//revive:disable:var-naming
package io

import (
	"encoding/binary"
	"fmt"
)

const MaxPCMBytes = 256 * 1024

func PCM16LE(pcm []int16) []byte {
	if len(pcm) == 0 {
		return nil
	}
	out := make([]byte, len(pcm)*2)
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(s)) // #nosec G115 -- PCM is two's complement 16-bit
	}
	return out
}

func FromPCM16LE(raw []byte) ([]int16, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw)%2 != 0 {
		return nil, fmt.Errorf("odd pcm byte length")
	}
	if len(raw) > MaxPCMBytes {
		return nil, fmt.Errorf("pcm frame larger than %d bytes", MaxPCMBytes)
	}
	out := make([]int16, len(raw)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(raw[i*2:])) // #nosec G115 -- PCM is two's complement 16-bit
	}
	return out, nil
}
