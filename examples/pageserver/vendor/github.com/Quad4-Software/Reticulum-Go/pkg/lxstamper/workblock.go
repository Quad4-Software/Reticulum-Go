// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package lxstamper

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/Quad4-Software/Reticulum-Go/pkg/cryptography"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// StampWorkblock returns the HKDF-expanded workblock (256 * expandRounds
// bytes) computed by the parallel CPU expand. Output is byte-identical to
// LXStamper.
func StampWorkblock(material []byte, expandRounds int) ([]byte, error) {
	return StampWorkblockCPU(material, expandRounds)
}

// StampWorkblockCPU forces the parallel CPU workblock path.
func StampWorkblockCPU(material []byte, expandRounds int) ([]byte, error) {
	if expandRounds <= 0 {
		return nil, errors.New("lxstamper: expandRounds must be positive")
	}
	if len(material) == 0 {
		return nil, errors.New("lxstamper: workblock material required")
	}
	return cpuExpandWorkblock(material, expandRounds)
}

func cpuExpandWorkblock(material []byte, expandRounds int) ([]byte, error) {
	out := make([]byte, 256*expandRounds)
	workers := max(min(runtime.GOMAXPROCS(0), expandRounds), 1)
	var (
		wg   sync.WaitGroup
		once sync.Once
		ret  error
	)
	chunk := (expandRounds + workers - 1) / workers
	for w := range workers {
		start := w * chunk
		if start >= expandRounds {
			break
		}
		end := min(start+chunk, expandRounds)
		wg.Go(func() {
			saltSrc := make([]byte, 0, len(material)+16)
			nBuf := make([]byte, 0, 16)
			for n := start; n < end; n++ {
				var err error
				nBuf, err = msgpack.AppendMarshal(nBuf[:0], n)
				if err != nil {
					once.Do(func() { ret = fmt.Errorf("lxstamper: workblock msgpack: %w", err) })
					return
				}
				saltSrc = append(saltSrc[:0], material...)
				saltSrc = append(saltSrc, nBuf...)
				saltSum := sha256.Sum256(saltSrc)
				dst := out[n*256 : (n+1)*256]
				if err := cryptography.DeriveKeyInto(dst, material, saltSum[:], nil); err != nil {
					once.Do(func() { ret = fmt.Errorf("lxstamper: workblock hkdf: %w", err) })
					return
				}
			}
		})
	}
	wg.Wait()
	return out, ret
}
