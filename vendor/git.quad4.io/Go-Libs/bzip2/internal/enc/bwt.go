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
type Scratch struct {
	sa, rank, tmp []int
	sa2           []int
	cnt           []int
	mtfv          []uint16
	yy            []byte
	mtfFreq       []int32
	selector      []uint8
	selectorMtf   []uint8
}

func (s *Scratch) grow(n int) {
	if cap(s.sa) >= n {
		s.sa = s.sa[:n]
		s.rank = s.rank[:n]
		s.tmp = s.tmp[:n]
	} else {
		s.sa = make([]int, n)
		s.rank = make([]int, n)
		s.tmp = make([]int, n)
	}
	if cap(s.sa2) < n {
		s.sa2 = make([]int, n)
	} else {
		s.sa2 = s.sa2[:n]
	}
	// Keys are byte values (0–255) on the first doubling step, then 0..n−1.
	buckets := maxInt(256, n) + 1
	if cap(s.cnt) < buckets {
		s.cnt = make([]int, buckets)
	} else {
		s.cnt = s.cnt[:buckets]
	}
}

// countingSortStableBySecondary sorts sa into dst by key rank[(sa[i]+k)%n] (stable).
func countingSortStableBySecondary(sa, dst []int, n, k int, rank []int, cnt []int) {
	B := maxInt(256, n)
	clear(cnt[:B+1])
	for i := 0; i < n; i++ {
		key := rank[(sa[i]+k)%n]
		cnt[key]++
	}
	for i := 1; i < B; i++ {
		cnt[i] += cnt[i-1]
	}
	for i := n - 1; i >= 0; i-- {
		key := rank[(sa[i]+k)%n]
		cnt[key]--
		dst[cnt[key]] = sa[i]
	}
}

// countingSortStableByPrimary sorts sa into dst by key rank[sa[i]] (stable).
func countingSortStableByPrimary(sa, dst []int, n int, rank []int, cnt []int) {
	B := maxInt(256, n)
	clear(cnt[:B+1])
	for i := 0; i < n; i++ {
		key := rank[sa[i]]
		cnt[key]++
	}
	for i := 1; i < B; i++ {
		cnt[i] += cnt[i-1]
	}
	for i := n - 1; i >= 0; i-- {
		key := rank[sa[i]]
		cnt[key]--
		dst[cnt[key]] = sa[i]
	}
}

// buildCyclicSuffixArray builds the sorted cyclic suffix array of block and returns the
// index of the original string (rotation starting at 0) in that order.
// Uses prefix doubling with O(n) radix passes per round (O(n log n) total), not sort.Slice.
func buildCyclicSuffixArray(block []byte, sc *Scratch) (sa []int, origPtr int) {
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

	for i := range sa {
		sa[i] = i
	}
	for i := range rank {
		rank[i] = int(block[i])
	}
	for k := 1; k < n; k *= 2 {
		countingSortStableBySecondary(sa, sa2, n, k, rank, cnt)
		countingSortStableByPrimary(sa2, sa, n, rank, cnt)
		tmp[sa[0]] = 0
		for i := 1; i < n; i++ {
			a, b := sa[i-1], sa[i]
			same := rank[a] == rank[b] && rank[(a+k)%n] == rank[(b+k)%n]
			tmp[sa[i]] = tmp[sa[i-1]]
			if !same {
				tmp[sa[i]]++
			}
		}
		copy(rank, tmp)
		if rank[sa[n-1]] == n-1 {
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
