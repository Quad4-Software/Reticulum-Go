// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package securemem

import (
	"crypto/rand"
	"sync"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
)

const (
	canarySize    = 16
	arenaDataSize = 256 * 1024
	maxClassSize  = 4096
	idleArenaKeep = 2
)

var sizeClasses = []int{64, 128, 256, 512, 1024, 2048, 4096}

func roundUp(n, to int) int {
	return (n + to - 1) / to * to
}

// classFor returns the smallest cell size that fits n, or -1 when n is too
// large for the slab pool and needs a dedicated mapping.
func classFor(n int) int {
	for _, c := range sizeClasses {
		if n <= c {
			return c
		}
	}
	return -1
}

// cellStride is the bytes a cell occupies: canary, payload, canary.
func cellStride(size int) int { return canarySize + size + canarySize }

type cellRef struct {
	ar         *arena
	idx        int
	canA, canB [canarySize]byte
}

type arena struct {
	mapping []byte // full mapping including guard pages (usable on fallback)
	data    []byte // usable inner region
	stride  int
	nCells  int
	free    []bool
	nFree   int
	locked  bool
}

var pool = struct {
	sync.Mutex
	arenas map[int][]*arena
}{arenas: make(map[int][]*arena)}

func newArena(class int) (*arena, error) {
	stride := cellStride(class)
	nCells := arenaDataSize / stride
	mapping, usable, err := mapSecret(nCells * stride)
	if err != nil {
		return nil, err
	}
	locked := true
	if err := lockRange(usable); err != nil {
		// Soft-fail: the buffer still works, Locked() reports false.
		debugLockDenied(err)
		locked = false
	}
	free := make([]bool, nCells)
	for i := range free {
		free[i] = true
	}
	return &arena{
		mapping: mapping,
		data:    usable,
		stride:  stride,
		nCells:  nCells,
		free:    free,
		nFree:   nCells,
		locked:  locked,
	}, nil
}

var mlockDeniedOnce sync.Once

func debugLockDenied(err error) {
	mlockDeniedOnce.Do(func() {
		debug.Log(debug.DebugVerbose, "securemem page lock denied, secrets remain mapped but unlocked", "error", err)
	})
}

// allocCell returns a payload slice of exactly class bytes inside an arena
// cell, with fresh random canaries written around it.
func allocCell(class int) (*cellRef, []byte, error) {
	pool.Lock()
	list := pool.arenas[class]
	var ar *arena
	for _, a := range list {
		if a.nFree > 0 {
			ar = a
			break
		}
	}
	if ar == nil {
		var err error
		ar, err = newArena(class)
		if err != nil {
			pool.Unlock()
			return nil, nil, err
		}
		pool.arenas[class] = append(list, ar)
	}
	idx := -1
	for i := 0; i < ar.nCells; i++ {
		if ar.free[i] {
			idx = i
			break
		}
	}
	ar.free[idx] = false
	ar.nFree--
	off := idx * ar.stride
	cell := ar.data[off : off+ar.stride]
	payload := cell[canarySize : canarySize+class]
	var canA, canB [canarySize]byte
	_, _ = rand.Read(canA[:])
	_, _ = rand.Read(canB[:])
	copy(cell[:canarySize], canA[:])
	copy(cell[canarySize+class:], canB[:])
	pool.Unlock()
	return &cellRef{ar: ar, idx: idx, canA: canA, canB: canB}, payload, nil
}

// freeCell verifies the cell's canaries, wipes the whole stride and returns
// the cell to the pool. Fully free arenas beyond a small reserve are
// unmapped so a burst does not pin RSS.
func freeCell(ref *cellRef, class int) error {
	ar := ref.ar
	off := ref.idx * ar.stride
	cell := ar.data[off : off+ar.stride]
	var canaryErr error
	if !equalBytes(cell[:canarySize], ref.canA[:]) || !equalBytes(cell[canarySize+class:], ref.canB[:]) {
		canaryErr = ErrCanary
	}
	WipeBytes(cell)

	pool.Lock()
	ar.free[ref.idx] = true
	ar.nFree++
	if ar.nFree == ar.nCells {
		free := 0
		for _, a := range pool.arenas[class] {
			if a.nFree == a.nCells {
				free++
			}
		}
		if free > idleArenaKeep {
			list := pool.arenas[class]
			for i, a := range list {
				if a == ar {
					pool.arenas[class] = append(list[:i], list[i+1:]...)
					break
				}
			}
			pool.Unlock()
			WipeBytes(ar.data)
			unlockRange(ar.data)
			unmapSecret(ar.mapping)
			return canaryErr
		}
	}
	pool.Unlock()
	return canaryErr
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// Stats reports pool occupancy for diagnostics and tests.
type Stats struct {
	Arenas      int
	CellsAlloc  int
	CellsFree   int
	MappedBytes int
	FreeArenas  int
}

func PoolStats() Stats {
	pool.Lock()
	defer pool.Unlock()
	var s Stats
	for _, list := range pool.arenas {
		s.Arenas += len(list)
		for _, a := range list {
			s.CellsAlloc += a.nCells - a.nFree
			s.CellsFree += a.nFree
			s.MappedBytes += len(a.mapping)
			if a.nFree == a.nCells {
				s.FreeArenas++
			}
		}
	}
	return s
}
