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

	"github.com/Quad4-Software/Reticulum-Go/pkg/cryptography"
)

const stopCheckInterval = 64

// GenerateStamp searches for a detached hash stamp of the given size whose
// sha256(workblock+stamp) is at or below target(cost). material is used
// directly as the workblock (expand_rounds=0 semantics); callers wanting HKDF
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

	workers := max(runtime.GOMAXPROCS(0), 1)
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

	select {
	case r := <-resCh:
		if r.stamp == nil {
			return nil, 0, ctx.Err()
		}
		return r.stamp, StampValue(material, r.stamp), nil
	case <-ctx.Done():
		<-resCh
		return nil, 0, ctx.Err()
	}
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

	workers := max(runtime.GOMAXPROCS(0), 1)
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

	select {
	case r := <-resCh:
		if r.pub == [32]byte{} && r.priv == [32]byte{} {
			return priv, pub, 0, ctx.Err()
		}
		return r.priv, r.pub, StampValue(material, r.pub[:]), nil
	case <-ctx.Done():
		<-resCh
		return priv, pub, 0, ctx.Err()
	}
}
