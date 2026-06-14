// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

package enc

// WriteStreamHeader writes the bzip2 magic bytes and ASCII digit for block size (1-9 x 100 KiB).
func WriteStreamHeader(w *BitWriter, blockSize100k int) {
	w.PutUChar(0x42)
	w.PutUChar(0x5a)
	w.PutUChar(0x68)
	w.PutUChar(byte('0' + blockSize100k)) // #nosec G115 -- blockSize100k is 1-9 (caller-validated)
}

// WriteBlock emits one compressed block: header, CRC, BWT index, and Huffman-coded MTF stream.
func WriteBlock(w *BitWriter, block []byte, inUse [256]bool, blockCRC uint32, scratch *Scratch) {
	w.PutUChar(0x31)
	w.PutUChar(0x41)
	w.PutUChar(0x59)
	w.PutUChar(0x26)
	w.PutUChar(0x53)
	w.PutUChar(0x59)
	w.PutUInt32(blockCRC)
	w.WriteBits(1, 0)
	sa, origPtr := buildCyclicSuffixArray(block, scratch)
	unseqToSeq, nInUse := makeUnseqToSeq(inUse)
	mtfv, mtfFreq := generateMTFValues(block, sa, unseqToSeq, nInUse, scratch)
	w.WriteBits(24, uint32(origPtr)) // #nosec G115 -- origPtr is BWT index in [0,len(block)); len(block) < 900001
	sendMTFValues(w, inUse, mtfv, mtfFreq, nInUse, scratch)
}

// WriteStreamTrailer writes the stream end marker and combined CRC.
func WriteStreamTrailer(w *BitWriter, combinedCRC uint32) {
	w.PutUChar(0x17)
	w.PutUChar(0x72)
	w.PutUChar(0x45)
	w.PutUChar(0x38)
	w.PutUChar(0x50)
	w.PutUChar(0x90)
	w.PutUInt32(combinedCRC)
	w.Finish()
}

// CombineCRC folds blockCRC into the running stream CRC (bzip2 combined CRC algorithm).
func CombineCRC(prev uint32, blockCRC uint32) uint32 {
	return (prev<<1 | prev>>31) ^ blockCRC
}
