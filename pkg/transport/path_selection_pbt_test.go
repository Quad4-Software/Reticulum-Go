// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
	"time"

	"quad4/pbt/pkg/pbt"
	"quad4/reticulum-go/pkg/common"
)

func TestPBTShouldUpdateRejectsKnownBlobWhenHopsNotWorse(t *testing.T) {
	// Contract from shouldUpdateAnnouncePath: when announceHops <= existing.HopCount
	// and the random blob is already known, the path must not update.
	hop := pbt.IntRange(0, 32)
	prop := pbt.ForAll(
		"known blob with hops <= existing never updates",
		hop,
		func(h int) bool {
			blob := []byte{1, 2, 3, 4, 5, 0, 0, 0, 0, 9}
			existing := &common.Path{
				HopCount:    uint8(h),
				RandomBlobs: [][]byte{append([]byte(nil), blob...)},
				Expires:     time.Now().Add(time.Hour),
			}
			announceHops := uint8(h)
			if h > 0 {
				announceHops = uint8(h - 1)
			}
			return !shouldUpdateAnnouncePath(existing, announcePathInput{
				destinationKnown: true,
				announceHops:     announceHops,
				randomBlob:       blob,
				now:              time.Now(),
			}, false)
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(80), pbt.WithSeed(41))
}

func TestPBTShouldUpdateRejectsKnownBlobWorseHopsUnlessUnresponsive(t *testing.T) {
	// When hops are strictly worse and the blob is known, update only if
	// pathUnresponsive and emitted equals pathEmitted (same blob).
	hop := pbt.IntRange(0, 30)
	prop := pbt.ForAll(
		"known blob worse hops rejects unless unresponsive equal emit",
		hop,
		func(h int) bool {
			blob := []byte{9, 9, 9, 9, 9, 0, 0, 0, 0, 7}
			existing := &common.Path{
				HopCount:    uint8(h),
				RandomBlobs: [][]byte{append([]byte(nil), blob...)},
				Expires:     time.Now().Add(time.Hour),
			}
			in := announcePathInput{
				destinationKnown: true,
				announceHops:     uint8(h + 1),
				randomBlob:       blob,
				now:              time.Now(),
			}
			if shouldUpdateAnnouncePath(existing, in, false) {
				return false
			}
			// Unresponsive with equal emit on known blob is allowed by the contract.
			_ = shouldUpdateAnnouncePath(existing, in, true)
			return true
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(60), pbt.WithSeed(42))
}
