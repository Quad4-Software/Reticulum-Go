// SPDX-License-Identifier: LicenseRef-Reticulum

package lxmf

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"git.quad4.io/libraries/msgpack/v5/pkg/msgpack"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/cryptography"
)

// gpuWorkblockMinRounds uses GPU for StampWorkblock only when expand cost is high
// enough that PCIe overhead is amortized (discovery's 20 rounds stays on CPU).
const gpuWorkblockMinRounds = 64

// StampWorkblock returns the HKDF-expanded workblock (256 * expandRounds bytes).
// Uses OpenCL when a GPU is available and expandRounds >= 64, otherwise a
// parallel CPU expand. Output is byte-identical to LXStamper.
func StampWorkblock(material []byte, expandRounds int) ([]byte, error) {
	if expandRounds <= 0 {
		return nil, errors.New("lxmf: expandRounds must be positive")
	}
	if len(material) == 0 {
		return nil, errors.New("lxmf: workblock material required")
	}

	ensureBackend()
	if PreferredStampBackend() != "cpu" && gpuEngine != nil && expandRounds >= gpuWorkblockMinRounds {
		out, err := gpuEngine.workblock(material, expandRounds)
		if err == nil {
			return out, nil
		}
		if PreferredStampBackend() == "gpu" {
			return nil, err
		}
	}
	return stampWorkblockCPU(material, expandRounds)
}

// StampWorkblockCPU forces the parallel CPU workblock path.
func StampWorkblockCPU(material []byte, expandRounds int) ([]byte, error) {
	if expandRounds <= 0 {
		return nil, errors.New("lxmf: expandRounds must be positive")
	}
	if len(material) == 0 {
		return nil, errors.New("lxmf: workblock material required")
	}
	return stampWorkblockCPU(material, expandRounds)
}

func stampWorkblockCPU(material []byte, expandRounds int) ([]byte, error) {
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
					once.Do(func() { ret = fmt.Errorf("lxmf: workblock msgpack: %w", err) })
					return
				}
				saltSrc = append(saltSrc[:0], material...)
				saltSrc = append(saltSrc, nBuf...)
				saltSum := sha256.Sum256(saltSrc)
				block, err := cryptography.DeriveKey(material, saltSum[:], nil, 256)
				if err != nil {
					once.Do(func() { ret = fmt.Errorf("lxmf: workblock hkdf: %w", err) })
					return
				}
				copy(out[n*256:(n+1)*256], block)
			}
		})
	}
	wg.Wait()
	return out, ret
}
