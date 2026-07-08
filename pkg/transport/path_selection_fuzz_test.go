// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func FuzzShouldUpdateAnnouncePath(f *testing.F) {
	seed := []byte{1, 2, 3, 4, 5, 0, 0, 0, 0, 9}
	f.Add(seed, uint8(2), true, false)
	f.Add(seed, uint8(8), true, true)

	f.Fuzz(func(t *testing.T, blob []byte, hops uint8, known bool, unresponsive bool) {
		if len(blob) != 10 {
			return
		}
		var existing *common.Path
		if known {
			existing = &common.Path{
				HopCount:    hops,
				RandomBlobs: [][]byte{append([]byte(nil), blob...)},
				Expires:     time.Now().Add(time.Hour),
			}
		}
		_ = shouldUpdateAnnouncePath(existing, announcePathInput{
			destinationKnown: known,
			announceHops:     hops,
			randomBlob:       blob,
			now:              time.Now(),
		}, unresponsive)
	})
}
