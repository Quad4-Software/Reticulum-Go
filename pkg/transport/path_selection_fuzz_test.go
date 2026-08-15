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
	f.Add(seed, uint8(0), false, false)

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
		got := shouldUpdateAnnouncePath(existing, announcePathInput{
			destinationKnown: known,
			announceHops:     hops,
			randomBlob:       blob,
			now:              time.Now(),
		}, unresponsive)

		if !known || existing == nil {
			if !got {
				t.Fatal("unknown destination must update")
			}
			return
		}
		// Known blob with hops <= existing must not update.
		if hops <= existing.HopCount && !got {
			return
		}
		if hops <= existing.HopCount && got {
			t.Fatal("known blob with hops <= existing must not update")
		}
	})
}
