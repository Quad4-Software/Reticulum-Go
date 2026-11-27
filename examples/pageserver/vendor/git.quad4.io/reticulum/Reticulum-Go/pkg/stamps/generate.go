// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package stamps

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/cryptography"
)

const stopCheckInterval = 64

// maxWorkers caps the parallel search workers used by stamp generation
// (Python Stamps.MAX_WORKERS). Zero means GOMAXPROCS.
var maxWorkers atomic.Int64

// SetMaxWorkers applies the [stamps] max_workers config: n > 0 caps workers,
// n <= 0 restores the GOMAXPROCS default.
func SetMaxWorkers(n int) {
	maxWorkers.Store(int64(n))
}

// WorkerCount returns the effective generation worker count. The base is
// GOMAXPROCS up to 12 cores and half above, like Python's job sizing.
// max_workers only lowers that base, it never raises it.
func WorkerCount() int {
	jobs := max(runtime.GOMAXPROCS(0), 1)
	if jobs > 12 {
		jobs = jobs / 2
	}
	if n := int(maxWorkers.Load()); n > 0 && n < jobs {
		jobs = n
	}
	return jobs
}

// GenerateStamp searches for a detached hash stamp of the given size whose
// sha256(workblock+stamp) is at or below target(cost). material is used
// directly as the workblock (expand_rounds=0 semantics). Callers wanting HKDF
// expansion should pass the output of StampWorkblock. Returns the stamp and
// its value.
func GenerateStamp(ctx context.Context, material []byte, cost float64, size int) ([]byte, float64, error) {
	if cost < 0 || cost > StampCostCap {
		return nil, 0, errors.New("stamps: invalid stamp cost")
	}
	if size <= 0 {
		return nil, 0, errors.New("stamps: invalid stamp size")
	}
	target := TargetFromCost(cost)

	type result struct {
		stamp []byte
	}
	resCh := make(chan result, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	workers := WorkerCount()
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			buf := make([]byte, len(material)+size)
			copy(buf, material)
			stamp := buf[len(material):]
			c := 0
			for {
				c++
				if c >= stopCheckInterval {
					c = 0
					if ctx.Err() != nil {
						return
					}
				}
				if _, err := rand.Read(stamp); err != nil {
					return
				}
				sum := sha256.Sum256(buf)
				if hashLE(target, sum[:]) {
					select {
					case resCh <- result{stamp: append([]byte{}, stamp...)}:
					default:
					}
					return
				}
			}
		})
	}

	go func() {
		wg.Wait()
		close(resCh)
	}()

	var res result
	var retErr error
	select {
	case r := <-resCh:
		res = r
		if r.stamp == nil {
			retErr = ctx.Err()
		}
	case <-ctx.Done():
		retErr = ctx.Err()
	}
	// Workers read material while copying it into their search buffers;
	// join them before returning so callers can wipe material safely.
	cancel()
	wg.Wait()
	if retErr != nil {
		return nil, 0, retErr
	}
	return res.stamp, StampValue(material, res.stamp), nil
}

// GenerateStampedKey generates an X25519 keypair whose public key carries the
// stamp value: it loops until sha256(material+pub) <= target(cost). The public
// key bytes are the stamp. Matches Stamps.generate_stamped_key.
func GenerateStampedKey(ctx context.Context, material []byte, cost float64) (priv, pub [32]byte, value float64, err error) {
	if cost < 0 || cost > StampCostCap {
		return priv, pub, 0, errors.New("stamps: invalid stamp cost")
	}
	target := TargetFromCost(cost)

	type result struct {
		priv, pub [32]byte
	}
	resCh := make(chan result, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	workers := WorkerCount()
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			buf := make([]byte, len(material)+32)
			copy(buf, material)
			pubb := buf[len(material):]
			c := 0
			for {
				if c == 0 {
					if ctx.Err() != nil {
						return
					}
				}
				c = (c + 1) % stopCheckInterval
				privb, pubBytes, kerr := cryptography.GenerateKeyPair()
				if kerr != nil || len(pubBytes) != 32 {
					return
				}
				copy(pubb, pubBytes)
				sum := sha256.Sum256(buf)
				if hashLE(target, sum[:]) {
					var r result
					copy(r.priv[:], privb)
					copy(r.pub[:], pubBytes)
					select {
					case resCh <- r:
					default:
					}
					return
				}
			}
		})
	}

	go func() {
		wg.Wait()
		close(resCh)
	}()

	var res result
	var retErr error
	select {
	case r := <-resCh:
		res = r
		if r.pub == [32]byte{} && r.priv == [32]byte{} {
			retErr = ctx.Err()
		}
	case <-ctx.Done():
		retErr = ctx.Err()
	}
	// Workers read material while copying it into their search buffers;
	// join them before returning so callers can wipe material safely.
	cancel()
	wg.Wait()
	if retErr != nil {
		return priv, pub, 0, retErr
	}
	return res.priv, res.pub, StampValue(material, res.pub[:]), nil
}
