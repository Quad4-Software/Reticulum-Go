// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

package enc

func sendMTFValues(w *BitWriter, inUse [256]bool, mtfv []uint16, mtfFreq []int32, nInUse int, sc *Scratch) {
	alphaSize := nInUse + 2
	nMTF := len(mtfv)

	lenPack := [BZMaxAlphaSize][4]uint32{}
	lenT := [BZNGroups][BZMaxAlphaSize]uint8{}
	rfreq := [BZNGroups][BZMaxAlphaSize]int32{}
	if cap(sc.selector) < BZMaxSelectors {
		sc.selector = make([]uint8, 0, BZMaxSelectors)
	}
	selector := sc.selector[:0]

	nGroups := 6
	switch {
	case nMTF < 200:
		nGroups = 2
	case nMTF < 600:
		nGroups = 3
	case nMTF < 1200:
		nGroups = 4
	case nMTF < 2400:
		nGroups = 5
	}

	nPart := nGroups
	remF := nMTF
	gs := 0
	for nPart > 0 {
		tFreq := remF / nPart
		ge := gs - 1
		aFreq := 0
		for aFreq < tFreq && ge < alphaSize-1 {
			ge++
			aFreq += int(mtfFreq[ge])
		}
		if ge > gs && nPart != nGroups && nPart != 1 && ((nGroups-nPart)%2 == 1) {
			aFreq -= int(mtfFreq[ge])
			ge--
		}
		for v := range alphaSize {
			if v >= gs && v <= ge {
				lenT[nPart-1][v] = BZLesserICost
			} else {
				lenT[nPart-1][v] = BZGreaterICost
			}
		}
		nPart--
		gs = ge + 1
		remF -= aFreq
	}

	fave := [BZNGroups]int{}
	cost := [BZNGroups]uint16{}

	for range BZNIters {
		_ = fave
		for t := 0; t < nGroups; t++ {
			fave[t] = 0
			for v := range alphaSize {
				rfreq[t][v] = 0
			}
		}
		if nGroups == 6 {
			for v := range alphaSize {
				lenPack[v][0] = uint32(lenT[1][v])<<16 | uint32(lenT[0][v]) // #nosec G602 -- v < alphaSize <= BZMaxAlphaSize; lenPack fixed size
				lenPack[v][1] = uint32(lenT[3][v])<<16 | uint32(lenT[2][v]) // #nosec G602 -- same bounds as lenPack[v][0]
				lenPack[v][2] = uint32(lenT[5][v])<<16 | uint32(lenT[4][v]) // #nosec G602 -- same bounds as lenPack[v][0]
			}
		}
		nSelectors := 0
		totc := 0
		gs := 0
		selector = selector[:0]
		for gs < nMTF {
			ge := gs + BZGSize - 1
			if ge >= nMTF {
				ge = nMTF - 1
			}
			for t := 0; t < nGroups; t++ {
				cost[t] = 0
			}
			if nGroups == 6 && ge-gs+1 == 50 {
				var cost01, cost23, cost45 uint32
				for nn := range 50 {
					icv := mtfv[gs+nn]
					cost01 += lenPack[icv][0]
					cost23 += lenPack[icv][1]
					cost45 += lenPack[icv][2]
				}
				cost[0] = uint16(cost01 & 0xffff)
				cost[1] = uint16(cost01 >> 16)
				cost[2] = uint16(cost23 & 0xffff)
				cost[3] = uint16(cost23 >> 16)
				cost[4] = uint16(cost45 & 0xffff)
				cost[5] = uint16(cost45 >> 16)
			} else {
				for i := gs; i <= ge; i++ {
					icv := mtfv[i]
					for t := 0; t < nGroups; t++ {
						cost[t] += uint16(lenT[t][icv])
					}
				}
			}
			bc := 999999999
			bt := -1
			for t := 0; t < nGroups; t++ {
				if int(cost[t]) < bc {
					bc = int(cost[t])
					bt = t
				}
			}
			totc += bc
			fave[bt]++                             // #nosec G602 -- bt is argmin over t in [0,nGroups); 2 <= nGroups <= 6
			selector = append(selector, uint8(bt)) // #nosec G115 -- bt in [0,nGroups)
			if nGroups == 6 && ge-gs+1 == 50 {
				for nn := range 50 {
					rfreq[bt][mtfv[gs+nn]]++ // #nosec G602 -- mtfv symbols < alphaSize; rfreq second dim is BZMaxAlphaSize
				}
			} else {
				for i := gs; i <= ge; i++ {
					rfreq[bt][mtfv[i]]++ // #nosec G602 -- same symbol bounds as mtfv[gs+nn]
				}
			}
			gs = ge + 1
			nSelectors++
		}
		_ = totc
		var hbuf [BZMaxAlphaSize]uint8
		var fbuf [BZMaxAlphaSize]int32
		for t := 0; t < nGroups; t++ {
			for v := range alphaSize {
				fbuf[v] = rfreq[t][v] // #nosec G602 -- v < alphaSize <= BZMaxAlphaSize; fbuf is BZMaxAlphaSize
			}
			hbMakeCodeLengths(hbuf[:alphaSize], fbuf[:alphaSize], alphaSize, BZMaxHuffLen)
			copy(lenT[t][:alphaSize], hbuf[:alphaSize])
		}
	}

	var pos [BZNGroups]byte
	for i := 0; i < nGroups; i++ {
		pos[i] = byte(i)
	}
	nsel := len(selector)
	if cap(sc.selectorMtf) < nsel {
		sc.selectorMtf = make([]uint8, nsel)
	} else {
		sc.selectorMtf = sc.selectorMtf[:nsel]
	}
	selectorMtf := sc.selectorMtf
	for i := range selector {
		llI := selector[i]
		j := 0
		tmp := pos[j]
		for llI != tmp {
			j++
			tmp2 := tmp
			tmp = pos[j]
			pos[j] = tmp2
		}
		pos[0] = tmp
		selectorMtf[i] = byte(j)
	}

	code := [BZNGroups][BZMaxAlphaSize]int32{}
	for t := 0; t < nGroups; t++ {
		minLen := 32
		maxLen := 0
		for i := range alphaSize {
			if int(lenT[t][i]) > maxLen {
				maxLen = int(lenT[t][i])
			}
			if int(lenT[t][i]) < minLen {
				minLen = int(lenT[t][i])
			}
		}
		hbuf := lenT[t][:alphaSize]
		cbuf := code[t][:alphaSize]
		hbAssignCodes(cbuf, hbuf, minLen, maxLen, alphaSize)
	}

	var inUse16 [16]bool
	for i := range 16 {
		for j := range 16 {
			if inUse[i*16+j] {
				inUse16[i] = true
			}
		}
	}
	for i := range 16 {
		if inUse16[i] {
			w.WriteBits(1, 1)
		} else {
			w.WriteBits(1, 0)
		}
	}
	for i := range 16 {
		if !inUse16[i] {
			continue
		}
		for j := range 16 {
			if inUse[i*16+j] {
				w.WriteBits(1, 1)
			} else {
				w.WriteBits(1, 0)
			}
		}
	}

	w.WriteBits(3, uint32(nGroups))           // #nosec G115 -- nGroups is 2-6
	w.WriteBits(15, uint32(len(selectorMtf))) // #nosec G115 -- selector count bounded by MTF length / group size
	for i := range selectorMtf {
		for j := 0; j < int(selectorMtf[i]); j++ {
			w.WriteBits(1, 1)
		}
		w.WriteBits(1, 0)
	}

	for t := 0; t < nGroups; t++ {
		curr := int(lenT[t][0])
		w.WriteBits(5, uint32(curr)) // #nosec G115 -- curr is Huffman code length in 0..BZMaxHuffLen
		for i := range alphaSize {
			for curr < int(lenT[t][i]) {
				w.WriteBits(2, 2)
				curr++
			}
			for curr > int(lenT[t][i]) {
				w.WriteBits(2, 3)
				curr--
			}
			w.WriteBits(1, 0)
		}
	}

	selCtr := 0
	gs = 0
	for gs < nMTF {
		ge := gs + BZGSize - 1
		if ge >= nMTF {
			ge = nMTF - 1
		}
		sel := int(selector[selCtr])
		if nGroups == 6 && ge-gs+1 == 50 {
			for nn := range 50 {
				mtfvI := mtfv[gs+nn]
				w.WriteBits(int(lenT[sel][mtfvI]), uint32(code[sel][mtfvI])) // #nosec G115 -- codes assigned by hbAssignCodes; widths match lenT
			}
		} else {
			for i := gs; i <= ge; i++ {
				icv := mtfv[i]
				w.WriteBits(int(lenT[sel][icv]), uint32(code[sel][icv])) // #nosec G115 -- same as batched WriteBits above
			}
		}
		gs = ge + 1
		selCtr++
	}
	sc.selector = selector
}
