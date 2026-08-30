// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package lxstamper

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding"
	"encoding/binary"
	"runtime"
	"sync"
	"sync/atomic"
)

const cpuCancelCheckMask = 0x3fff // every 16384 attempts

func cpuMineStamp(ctx context.Context, workblock []byte, stampCost int) ([]byte, int, error) {
	target := stampTarget(stampCost)
	ms, err := midstateOfPrefix(workblock)
	if err != nil || len(ms.Marshaled) == 0 {
		return cpuMineStampFallback(ctx, workblock, stampCost)
	}
	midState := ms.Marshaled

	workers := max(runtime.GOMAXPROCS(0), 1)
	var found atomic.Bool
	resCh := make(chan []byte, 1)

	var seed [16]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, 0, err
	}

	var wg sync.WaitGroup
	for w := range workers {
		worker := w
		wg.Go(func() {
			h := sha256.New()
			un, ok := h.(encoding.BinaryUnmarshaler)
			if !ok {
				return
			}
			var stamp [32]byte
			copy(stamp[:16], seed[:])
			binary.BigEndian.PutUint32(stamp[12:16], uint32(worker)) // #nosec G115 -- worker index from GOMAXPROCS
			var sum [32]byte
			var counter uint64
			for !found.Load() {
				counter++
				if counter&cpuCancelCheckMask == 0 {
					select {
					case <-ctx.Done():
						return
					default:
					}
					if found.Load() {
						return
					}
				}
				binary.BigEndian.PutUint64(stamp[16:], counter)
				binary.BigEndian.PutUint32(stamp[24:], uint32(counter>>32)^uint32(worker)*0x9e3779b9) // #nosec G115 -- stamp mixing truncates to 32 bits
				binary.BigEndian.PutUint32(stamp[28:], uint32(counter)^uint32(worker)<<16)            // #nosec G115 -- stamp mixing truncates to 32 bits

				if err := un.UnmarshalBinary(midState); err != nil {
					return
				}
				h.Write(stamp[:])
				h.Sum(sum[:0])
				if bytes.Compare(sum[:], target[:]) > 0 {
					continue
				}
				if found.CompareAndSwap(false, true) {
					out := append([]byte(nil), stamp[:]...)
					select {
					case resCh <- out:
					default:
					}
				}
				return
			}
		})
	}

	go func() {
		wg.Wait()
		close(resCh)
	}()

	select {
	case stamp, ok := <-resCh:
		if !ok || stamp == nil {
			return nil, 0, ErrStampNotFound
		}
		return stamp, StampValue(workblock, stamp), nil
	case <-ctx.Done():
		found.Store(true)
		return nil, 0, ErrStampNotFound
	}
}

func cpuMineStampFallback(ctx context.Context, workblock []byte, stampCost int) ([]byte, int, error) {
	target := stampTarget(stampCost)
	workers := max(runtime.GOMAXPROCS(0), 1)
	var found atomic.Bool
	resCh := make(chan []byte, 1)

	var seed [16]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, 0, err
	}

	base := sha256.New()
	base.Write(workblock)
	mar, ok := base.(encoding.BinaryMarshaler)
	if !ok {
		return nil, 0, ErrStampNotFound
	}
	midState, err := mar.MarshalBinary()
	if err != nil {
		return nil, 0, err
	}

	var wg sync.WaitGroup
	for w := range workers {
		worker := w
		wg.Go(func() {
			h := sha256.New()
			un, ok := h.(encoding.BinaryUnmarshaler)
			if !ok {
				return
			}
			var stamp [32]byte
			copy(stamp[:16], seed[:])
			binary.BigEndian.PutUint32(stamp[12:16], uint32(worker)) // #nosec G115 -- worker index from GOMAXPROCS
			var sum [32]byte
			var counter uint64
			for !found.Load() {
				counter++
				if counter&cpuCancelCheckMask == 0 {
					select {
					case <-ctx.Done():
						return
					default:
					}
				}
				binary.BigEndian.PutUint64(stamp[16:], counter)
				binary.BigEndian.PutUint32(stamp[24:], uint32(counter>>32)^uint32(worker)*0x9e3779b9) // #nosec G115 -- stamp mixing truncates to 32 bits
				binary.BigEndian.PutUint32(stamp[28:], uint32(counter)^uint32(worker)<<16)            // #nosec G115 -- stamp mixing truncates to 32 bits
				if err := un.UnmarshalBinary(midState); err != nil {
					return
				}
				h.Write(stamp[:])
				h.Sum(sum[:0])
				if bytes.Compare(sum[:], target[:]) > 0 {
					continue
				}
				if found.CompareAndSwap(false, true) {
					out := append([]byte(nil), stamp[:]...)
					select {
					case resCh <- out:
					default:
					}
				}
				return
			}
		})
	}
	go func() {
		wg.Wait()
		close(resCh)
	}()
	select {
	case stamp, ok := <-resCh:
		if !ok || stamp == nil {
			return nil, 0, ErrStampNotFound
		}
		return stamp, StampValue(workblock, stamp), nil
	case <-ctx.Done():
		found.Store(true)
		return nil, 0, ErrStampNotFound
	}
}
