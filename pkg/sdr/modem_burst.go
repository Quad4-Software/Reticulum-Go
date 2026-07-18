// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sdr

import (
	"encoding/binary"
	"hash/crc32"
	"sync"
)

const (
	burstPreamble0     byte = 0x2D
	burstPreamble1     byte = 0xD4
	burstMaxPayload         = 2048
	burstSamplesPerBit      = 8
	burstMinFrameBytes      = 2 + 2 + 4
)

var burstBitBufPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, 4096)
	},
}

// BurstModem maps RNS frames to baseband IQ bursts and back.
//
// Wire bytes before BPSK:
//
//	preamble 2B | length uint16 BE | payload | crc32 IEEE
//
// Modulation is absolute BPSK with SamplesPerBit samples per bit.
type BurstModem struct {
	SamplesPerBit int
}

// NewBurstModem returns a modem with default samples per bit.
func NewBurstModem() *BurstModem {
	return &BurstModem{SamplesPerBit: burstSamplesPerBit}
}

func (m *BurstModem) spb() int {
	if m == nil || m.SamplesPerBit <= 0 {
		return burstSamplesPerBit
	}
	return m.SamplesPerBit
}

// Encode turns payload into IQ samples.
func (m *BurstModem) Encode(payload []byte) ([]Complex64, error) {
	if len(payload) > burstMaxPayload {
		return nil, errBurstTooLarge
	}
	frameLen := burstMinFrameBytes + len(payload)
	frame := make([]byte, frameLen)
	frame[0] = burstPreamble0
	frame[1] = burstPreamble1
	binary.BigEndian.PutUint16(frame[2:4], uint16(len(payload))) // #nosec G115 -- payload capped at burstMaxPayload
	copy(frame[4:], payload)
	crc := crc32.ChecksumIEEE(payload)
	binary.BigEndian.PutUint32(frame[4+len(payload):], crc)

	spb := m.spb()
	out := make([]Complex64, frameLen*8*spb)
	idx := 0
	for _, b := range frame {
		for bit := 7; bit >= 0; bit-- {
			phase := float32(1)
			if (b>>uint(bit))&1 == 1 {
				phase = -1
			}
			for range spb {
				out[idx] = Complex64{I: phase, Q: 0}
				idx++
			}
		}
	}
	return out, nil
}

// Decode recovers a payload from IQ samples. Returns false when no valid frame is found.
func (m *BurstModem) Decode(samples []Complex64) ([]byte, bool) {
	bits := m.demodBits(samples)
	if bits == nil {
		return nil, false
	}
	defer burstPutBits(bits)

	minBits := burstMinFrameBytes * 8
	if len(bits) < minBits {
		return nil, false
	}
	for start := 0; start+minBits <= len(bits); start++ {
		if !burstMatchPreamble(bits, start) {
			continue
		}
		n, ok := burstReadUint16(bits, start+16)
		if !ok || n > burstMaxPayload {
			continue
		}
		needBits := (burstMinFrameBytes + n) * 8
		if start+needBits > len(bits) {
			continue
		}
		payloadOff := start + 32
		crcOff := payloadOff + n*8
		want, ok := burstReadUint32(bits, crcOff)
		if !ok {
			continue
		}
		payload := make([]byte, n)
		burstBitsToBytesAt(bits, payloadOff, payload)
		if crc32.ChecksumIEEE(payload) != want {
			continue
		}
		return payload, true
	}
	return nil, false
}

func burstMatchPreamble(bits []byte, start int) bool {
	if start+16 > len(bits) {
		return false
	}
	b0, ok := burstReadByte(bits, start)
	if !ok || b0 != burstPreamble0 {
		return false
	}
	b1, ok := burstReadByte(bits, start+8)
	return ok && b1 == burstPreamble1
}

func burstReadByte(bits []byte, start int) (byte, bool) {
	if start+8 > len(bits) {
		return 0, false
	}
	var b byte
	for bit := range 8 {
		b <<= 1
		if bits[start+bit] != 0 {
			b |= 1
		}
	}
	return b, true
}

func burstReadUint16(bits []byte, start int) (int, bool) {
	hi, ok := burstReadByte(bits, start)
	if !ok {
		return 0, false
	}
	lo, ok := burstReadByte(bits, start+8)
	if !ok {
		return 0, false
	}
	return int(hi)<<8 | int(lo), true
}

func burstReadUint32(bits []byte, start int) (uint32, bool) {
	if start+32 > len(bits) {
		return 0, false
	}
	var v uint32
	for i := range 4 {
		b, ok := burstReadByte(bits, start+i*8)
		if !ok {
			return 0, false
		}
		v = v<<8 | uint32(b)
	}
	return v, true
}

func burstBitsToBytesAt(bits []byte, start int, out []byte) {
	for i := range out {
		b, _ := burstReadByte(bits, start+i*8)
		out[i] = b
	}
}

func (m *BurstModem) demodBits(samples []Complex64) []byte {
	spb := m.spb()
	if len(samples) < spb {
		return nil
	}
	nBits := len(samples) / spb
	bits := burstGetBits(nBits)
	for i := range nBits {
		var sum float32
		base := i * spb
		end := base + spb
		for s := base; s < end; s++ {
			sum += samples[s].I
		}
		if sum < 0 {
			bits[i] = 1
		}
	}
	return bits
}

func burstGetBits(n int) []byte {
	buf := burstBitBufPool.Get().([]byte)
	if cap(buf) < n {
		return make([]byte, n)
	}
	buf = buf[:n]
	clear(buf)
	return buf
}

func burstPutBits(bits []byte) {
	if bits == nil || cap(bits) == 0 {
		return
	}
	burstBitBufPool.Put(bits[:0])
}

var errBurstTooLarge = errBurstSize{}

type errBurstSize struct{}

func (errBurstSize) Error() string { return "burst payload exceeds max size" }
