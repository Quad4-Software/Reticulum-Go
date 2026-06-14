// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

package enc

// EncodedBitBufferCap returns a conservative output byte capacity so one compressed block rarely
// forces the internal slice to realloc during streaming (worst-case blocks can exceed input length).
func EncodedBitBufferCap(nblockMax int) int {
	if nblockMax <= 0 {
		return 0
	}
	return nblockMax*3 + 256*1024
}

// BitWriter buffers a bzip2 bit stream into a byte slice (MSB-first within each byte, per format).
type BitWriter struct {
	out    []byte
	bsBuff uint32
	bsLive int
}

// Bytes returns the encoded byte stream built so far (including any bit padding after Finish).
func (w *BitWriter) Bytes() []byte { return w.out }

// ResetOutput clears the byte buffer after a successful drain to an io.Writer.
// Bit accumulator state (bsBuff, bsLive) is unchanged.
func (w *BitWriter) ResetOutput() {
	w.out = w.out[:0]
}

// ResetForNewStream clears output and partial-bit state before starting another bzip2 stream.
func (w *BitWriter) ResetForNewStream() {
	w.out = w.out[:0]
	w.bsBuff = 0
	w.bsLive = 0
}

// Grow reserves at least n bytes of capacity in the output buffer.
func (w *BitWriter) Grow(n int) {
	if n <= cap(w.out) {
		return
	}
	if cap(w.out) == 0 {
		w.out = make([]byte, 0, n)
		return
	}
	if n < cap(w.out)*2 {
		n = cap(w.out) * 2
	}
	nw := make([]byte, len(w.out), n)
	copy(nw, w.out)
	w.out = nw
}

func (w *BitWriter) needW(n int) {
	for w.bsLive >= 8 {
		w.out = append(w.out, byte(w.bsBuff>>24))
		w.bsBuff <<= 8
		w.bsLive -= 8
	}
	_ = n
}

// WriteBits appends the low n bits of v to the stream (n must not exceed remaining space in the algorithm).
func (w *BitWriter) WriteBits(n int, v uint32) {
	w.needW(n)
	w.bsBuff |= v << (32 - w.bsLive - n)
	w.bsLive += n
}

// PutUChar writes one byte (8 bits).
func (w *BitWriter) PutUChar(c byte) {
	w.WriteBits(8, uint32(c))
}

// PutUInt32 writes u as four big-endian bytes.
func (w *BitWriter) PutUInt32(u uint32) {
	w.WriteBits(8, (u>>24)&0xff)
	w.WriteBits(8, (u>>16)&0xff)
	w.WriteBits(8, (u>>8)&0xff)
	w.WriteBits(8, u&0xff)
}

// Finish aligns the bit stream to a byte boundary (pads with zeros as required by the format).
func (w *BitWriter) Finish() {
	for w.bsLive > 0 {
		w.out = append(w.out, byte(w.bsBuff>>24))
		w.bsBuff <<= 8
		if w.bsLive >= 8 {
			w.bsLive -= 8
		} else {
			w.bsLive = 0
		}
	}
}
