// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

package enc

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Scratch holds reusable buffers for block encoding (BWT and MTF stages).
// Suffix-sort buffers use int32 rather than int: block lengths are capped well under
// 2^31 (max ~900000), and the narrower width halves the memory traffic of the
// counting-sort passes below, which are the hottest and most cache-sensitive loops
// in the encoder.
type Scratch struct {
	sa, rank, tmp []int32
	sa2           []int32
	cnt           []int32
	keys          []int32
	mtfv          []uint16
	yy            []byte
	mtfFreq       []int32
	selector      []uint8
	selectorMtf   []uint8
}

// PrepareEncoderAux pre-sizes auxiliary buffers reused across blocks (selector MTF streams,
// symbol frequency slices). Suffix-sort and MTF vectors still grow lazily by block length.
func (s *Scratch) PrepareEncoderAux() {
	if cap(s.mtfFreq) < BZMaxAlphaSize {
		s.mtfFreq = make([]int32, BZMaxAlphaSize)
	}
	if cap(s.yy) < 256 {
		s.yy = make([]byte, 256)
	}
	if cap(s.selector) < BZMaxSelectors {
		s.selector = make([]uint8, 0, BZMaxSelectors)
	}
	if cap(s.selectorMtf) < BZMaxSelectors {
		s.selectorMtf = make([]uint8, BZMaxSelectors)
	}
}

func (s *Scratch) grow(n int) {
	if cap(s.sa) >= n {
		s.sa = s.sa[:n]
	} else {
		s.sa = make([]int32, n)
	}
	// rank/tmp are kept at double length: rank[n+i] mirrors rank[i]. This lets every
	// lookup of rank[(sa[i]+k)%n] become a branchless rank[sa[i]+k], since sa[i]+k never
	// exceeds 2n-2 for k < n.
	need2n := 2 * n
	if cap(s.rank) >= need2n {
		s.rank = s.rank[:need2n]
	} else {
		s.rank = make([]int32, need2n)
	}
	if cap(s.tmp) >= need2n {
		s.tmp = s.tmp[:need2n]
	} else {
		s.tmp = make([]int32, need2n)
	}
	if cap(s.sa2) < n {
		s.sa2 = make([]int32, n)
	} else {
		s.sa2 = s.sa2[:n]
	}
	if cap(s.keys) < n {
		s.keys = make([]int32, n)
	} else {
		s.keys = s.keys[:n]
	}
	// Keys are byte values (0–255) on the first doubling step, then 0..n−1.
	buckets := maxInt(256, n) + 1
	if cap(s.cnt) < buckets {
		s.cnt = make([]int32, buckets)
	} else {
		s.cnt = s.cnt[:buckets]
	}
}

// countingSortStableBySecondary sorts sa into dst by key rank[(sa[i]+k)%n] (stable).
// rank must be mirrored (rank[n+i] == rank[i]) so the modulo can be dropped. keys is
// scratch space that caches each element's key so the placement pass reads it back
// sequentially instead of re-chasing the sa[i] -> rank[sa[i]+k] pointer chain.
func countingSortStableBySecondary(sa, dst []int32, n int, k int32, rank []int32, cnt []int32, keys []int32) {
	B := maxInt(256, n)
	clear(cnt[:B+1])
	for i := range n {
		key := rank[sa[i]+k]
		keys[i] = key
		cnt[key]++
	}
	for i := 1; i < B; i++ {
		cnt[i] += cnt[i-1]
	}
	for i := n - 1; i >= 0; i-- {
		key := keys[i]
		cnt[key]--
		dst[cnt[key]] = sa[i]
	}
}

// countingSortStableByPrimary sorts sa into dst by key rank[sa[i]] (stable).
// See countingSortStableBySecondary for why keys is cached rather than recomputed.
func countingSortStableByPrimary(sa, dst []int32, n int, rank []int32, cnt []int32, keys []int32) {
	B := maxInt(256, n)
	clear(cnt[:B+1])
	for i := range n {
		key := rank[sa[i]]
		keys[i] = key
		cnt[key]++
	}
	for i := 1; i < B; i++ {
		cnt[i] += cnt[i-1]
	}
	for i := n - 1; i >= 0; i-- {
		key := keys[i]
		cnt[key]--
		dst[cnt[key]] = sa[i]
	}
}

// buildCyclicSuffixArray builds the sorted cyclic suffix array of block and returns the
// index of the original string (rotation starting at 0) in that order.
// Uses prefix doubling with O(n) radix passes per round (O(n log n) total), not sort.Slice.
func buildCyclicSuffixArray(block []byte, sc *Scratch) (sa []int32, origPtr int) {
	n := len(block)
	if n == 0 {
		return nil, 0
	}
	sc.grow(n)
	sa = sc.sa
	sa2 := sc.sa2
	rank := sc.rank
	tmp := sc.tmp
	cnt := sc.cnt
	keys := sc.keys

	for i := range sa {
		sa[i] = int32(i) // #nosec G115 -- i < n, block length capped well under 2^31
	}
	for i := range n {
		rank[i] = int32(block[i])
	}
	copy(rank[n:2*n], rank[:n])

	nMinus1 := int32(n - 1) // #nosec G115 -- n capped well under 2^31
	for k := 1; k < n; k *= 2 {
		kk := int32(k) // #nosec G115 -- k < n, capped well under 2^31
		countingSortStableBySecondary(sa, sa2, n, kk, rank, cnt, keys)
		countingSortStableByPrimary(sa2, sa, n, rank, cnt, keys)
		tmp[sa[0]] = 0
		for i := 1; i < n; i++ {
			a, b := sa[i-1], sa[i]
			same := rank[a] == rank[b] && rank[a+kk] == rank[b+kk]
			tmp[sa[i]] = tmp[sa[i-1]]
			if !same {
				tmp[sa[i]]++
			}
		}
		copy(tmp[n:2*n], tmp[:n])
		rank, tmp = tmp, rank
		if rank[sa[n-1]] == nMinus1 {
			break
		}
	}
	for i := range sa {
		if sa[i] == 0 {
			origPtr = i
			break
		}
	}
	return sa, origPtr
}
