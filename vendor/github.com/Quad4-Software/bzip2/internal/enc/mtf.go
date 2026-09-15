// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

package enc

import "bytes"

func makeUnseqToSeq(inUse [256]bool) (unseqToSeq [256]byte, nInUse int) {
	for i := range 256 {
		if inUse[i] {
			unseqToSeq[i] = byte(nInUse)
			nInUse++
		}
	}
	return unseqToSeq, nInUse
}

// mtfIndexAndMove finds llI's position in yy and moves it to the front, shifting the
// preceding entries back by one. It uses bytes.IndexByte for the search, which is
// vectorized by the Go runtime and considerably faster than a byte-at-a-time scan,
// since this runs once per input byte and yy can hold up to 256 entries.
func mtfIndexAndMove(yy []byte, llI byte) int {
	j := bytes.IndexByte(yy, llI)
	if j <= 0 {
		return 0
	}
	sym := yy[j]
	copy(yy[1:j+1], yy[0:j])
	yy[0] = sym
	return j
}

func generateMTFValues(block []byte, sa []int32, unseqToSeq [256]byte, nInUse int, sc *Scratch) (mtfv []uint16, mtfFreq []int32) {
	n := len(block)
	needCap := n + 2 + 2*n
	if cap(sc.mtfFreq) < BZMaxAlphaSize {
		sc.mtfFreq = make([]int32, BZMaxAlphaSize)
	} else {
		sc.mtfFreq = sc.mtfFreq[:BZMaxAlphaSize]
		clear(sc.mtfFreq)
	}
	mtfFreq = sc.mtfFreq
	if cap(sc.mtfv) < needCap {
		sc.mtfv = make([]uint16, 0, needCap)
	} else {
		sc.mtfv = sc.mtfv[:0]
	}
	mtfv = sc.mtfv
	if cap(sc.yy) < nInUse {
		sc.yy = make([]byte, nInUse)
	} else {
		sc.yy = sc.yy[:nInUse]
	}
	yy := sc.yy
	for i := range nInUse {
		yy[i] = byte(i)
	}
	eob := nInUse + 1
	zPend := 0
	nMinus1 := int32(n - 1) // #nosec G115 -- n capped well under 2^31
	for i := range n {
		j := sa[i] - 1
		if j < 0 {
			j = nMinus1
		}
		llI := unseqToSeq[block[j]]
		if yy[0] == llI {
			zPend++
			continue
		}
		if zPend > 0 {
			zPend--
			for {
				if zPend&1 != 0 {
					mtfv = append(mtfv, BZRunB)
					mtfFreq[BZRunB]++
				} else {
					mtfv = append(mtfv, BZRunA)
					mtfFreq[BZRunA]++
				}
				if zPend < 2 {
					break
				}
				zPend = (zPend - 2) / 2
			}
			zPend = 0
		}
		jj := mtfIndexAndMove(yy, llI)
		mtfv = append(mtfv, uint16(jj+1)) // #nosec G115 -- jj < nInUse and nInUse <= 256
		mtfFreq[jj+1]++
	}
	if zPend > 0 {
		zPend--
		for {
			if zPend&1 != 0 {
				mtfv = append(mtfv, BZRunB)
				mtfFreq[BZRunB]++
			} else {
				mtfv = append(mtfv, BZRunA)
				mtfFreq[BZRunA]++
			}
			if zPend < 2 {
				break
			}
			zPend = (zPend - 2) / 2
		}
	}
	mtfv = append(mtfv, uint16(eob)) // #nosec G115 -- eob = nInUse+1, nInUse <= 256
	mtfFreq[eob]++
	return mtfv, mtfFreq
}
