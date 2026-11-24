// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

package enc

// RLEEncoder performs bzip2 run-length encoding and tracks per-block CRC and symbol usage.
type RLEEncoder struct {
	Block      []byte
	InUse      [256]bool
	BlockCRC   uint32
	StateInCh  uint32
	StateInLen int
	NBlockMax  int
}

// ResetStream clears the encoder for a new stream (first block).
func (e *RLEEncoder) ResetStream() {
	e.Block = e.Block[:0]
	e.BlockCRC = 0xffffffff
	e.StateInCh = 256
	e.StateInLen = 0
	e.InUse = [256]bool{}
}

// StartBlock begins a new block after the previous one was flushed.
func (e *RLEEncoder) StartBlock() {
	e.Block = e.Block[:0]
	e.BlockCRC = 0xffffffff
	e.InUse = [256]bool{}
}

func addPairToBlock(block []byte, inUse *[256]bool, crc *uint32, stateInCh uint32, stateInLen int) []byte {
	ch := byte(stateInCh) // #nosec G115 -- stateInCh is a byte value or sentinel 256; callers only pass <256 here
	for range stateInLen {
		*crc = bzUpdateCRC(*crc, ch)
	}
	inUse[stateInCh] = true
	switch stateInLen {
	case 1:
		block = append(block, ch)
	case 2:
		block = append(block, ch, ch)
	case 3:
		block = append(block, ch, ch, ch)
	default:
		inUse[stateInLen-4] = true
		block = append(block, ch, ch, ch, ch, byte(stateInLen-4)) // #nosec G115 -- default branch: stateInLen is 4-255
	}
	return block
}

// FlushRL flushes the current RLE run into the block buffer.
func (e *RLEEncoder) FlushRL() {
	if e.StateInCh < 256 {
		e.Block = addPairToBlock(e.Block, &e.InUse, &e.BlockCRC, e.StateInCh, e.StateInLen)
	}
	e.StateInCh = 256
	e.StateInLen = 0
}

// AddByte ingests one input byte. If the block is full, consumed is false or blockFull is true.
func (e *RLEEncoder) AddByte(zch byte) (consumed bool, blockFull bool) {
	if len(e.Block) >= e.NBlockMax {
		return false, true
	}
	zchh := uint32(zch)
	if zchh != e.StateInCh && e.StateInLen == 1 {
		ch := byte(e.StateInCh) // #nosec G115 -- StateInCh is a byte value here (see zchh != branch)
		e.BlockCRC = bzUpdateCRC(e.BlockCRC, ch)
		e.InUse[e.StateInCh] = true
		e.Block = append(e.Block, ch)
		e.StateInCh = zchh
		return true, len(e.Block) >= e.NBlockMax
	}
	if zchh != e.StateInCh || e.StateInLen == 255 {
		if e.StateInCh < 256 {
			e.Block = addPairToBlock(e.Block, &e.InUse, &e.BlockCRC, e.StateInCh, e.StateInLen)
		}
		e.StateInCh = zchh
		e.StateInLen = 1
	} else {
		e.StateInLen++
	}
	return true, len(e.Block) >= e.NBlockMax
}

// AddBytes ingests as much of p as fits into the current block and run state.
// It returns the count consumed from p and whether the block is now full.
func (e *RLEEncoder) AddBytes(p []byte) (n int, blockFull bool) {
	for n < len(p) {
		consumed, full := e.AddByte(p[n])
		if !consumed {
			return n, true
		}
		n++
		if full {
			return n, true
		}
	}
	return n, false
}

// FinalCRC returns the CRC of the current block per bzip2 (bitwise complement of internal state).
func (e *RLEEncoder) FinalCRC() uint32 {
	return ^e.BlockCRC
}

// EncodeRLEBlock runs the RLE stage on raw up to a maximum block size (for tests and tooling).
func EncodeRLEBlock(raw []byte, nblockMax int) (block []byte, inUse [256]bool, blockCRC uint32) {
	var enc RLEEncoder
	enc.NBlockMax = nblockMax
	enc.ResetStream()
	for _, b := range raw {
		consumed, full := enc.AddByte(b)
		if !consumed {
			break
		}
		if full {
			break
		}
	}
	enc.FlushRL()
	return enc.Block, enc.InUse, enc.FinalCRC()
}
