// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

package enc

func hbMakeCodeLengths(lenOut []uint8, freq []int32, alphaSize int, maxLen int) {
	var weight [BZMaxAlphaSize*2 + 2]uint32
	var parent [BZMaxAlphaSize*2 + 2]int32
	var heap [BZMaxAlphaSize + 2]int32
	w := weight[:alphaSize*2+2]
	par := parent[:alphaSize*2+2]
	h := heap[:alphaSize+2]

	for {
		for i := range alphaSize {
			wv := freq[i]
			if wv == 0 {
				wv = 1
			}
			w[i+1] = uint32(wv) << 8 // #nosec G602 -- i in [0,alphaSize); w is sized alphaSize*2+2
		}

		nNodes := alphaSize
		nHeap := 0
		h[0] = 0
		w[0] = 0
		par[0] = -2

		for i := 1; i <= alphaSize; i++ {
			par[i] = -1
			nHeap++
			h[nHeap] = int32(i)
			upheap(h, w, nHeap)
		}

		for nHeap > 1 {
			n1 := h[1]
			h[1] = h[nHeap]
			nHeap--
			downheap(h, w, nHeap)
			n2 := h[1]
			h[1] = h[nHeap]
			nHeap--
			downheap(h, w, nHeap)
			nNodes++
			par[n1] = int32(nNodes) // #nosec G115 -- Huffman tree node index bounded by 2*alphaSize+1
			par[n2] = int32(nNodes) // #nosec G115 -- Huffman tree node index bounded by 2*alphaSize+1
			w1, w2 := w[n1], w[n2]
			d1, d2 := int(w1&0xff), int(w2&0xff)
			md := max(d2, d1)
			dep := 1 + md
			w[nNodes] = (w1 & 0xffffff00) + (w2 & 0xffffff00) | uint32(dep)
			par[nNodes] = -1
			nHeap++
			h[nHeap] = int32(nNodes) // #nosec G115 -- same node index bound as parent assignments
			upheap(h, w, nHeap)
		}

		tooLong := false
		for i := 1; i <= alphaSize; i++ {
			j := 0
			k := i
			for par[k] >= 0 {
				k = int(par[k])
				j++
			}
			lenOut[i-1] = uint8(j)
			if j > maxLen {
				tooLong = true
			}
		}
		if !tooLong {
			break
		}
		for i := 1; i <= alphaSize; i++ {
			j := int(w[i] >> 8)
			j = 1 + (j / 2)
			w[i] = uint32(j) << 8
		}
	}
}

func upheap(heap []int32, weight []uint32, zz int) {
	tmp := heap[zz]
	for zz > 1 && weight[tmp] < weight[heap[zz>>1]] {
		heap[zz] = heap[zz>>1]
		zz >>= 1
	}
	heap[zz] = tmp
}

func downheap(heap []int32, weight []uint32, nHeap int) {
	zz := 1
	tmp := heap[1]
	for {
		yy := zz << 1
		if yy > nHeap {
			break
		}
		if yy < nHeap && weight[heap[yy+1]] < weight[heap[yy]] {
			yy++
		}
		if weight[tmp] < weight[heap[yy]] {
			break
		}
		heap[zz] = heap[yy]
		zz = yy
	}
	heap[zz] = tmp
}

func hbAssignCodes(code []int32, length []uint8, minLen, maxLen, alphaSize int) {
	vec := int32(0)
	for n := minLen; n <= maxLen; n++ {
		for i := range alphaSize {
			if int(length[i]) == n {
				code[i] = vec
				vec++
			}
		}
		vec <<= 1
	}
}
