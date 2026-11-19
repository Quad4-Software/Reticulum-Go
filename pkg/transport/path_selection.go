// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
)

const (
	maxRandomBlobs = 64
)

func announceEmitted(randomBlob []byte) uint32 {
	if len(randomBlob) < 10 {
		return 0
	}
	var emitted uint32
	for _, b := range randomBlob[5:10] {
		emitted = (emitted << 8) | uint32(b)
	}
	return emitted
}

func timebaseFromRandomBlobs(blobs [][]byte) uint32 {
	var base uint32
	for _, blob := range blobs {
		if emitted := announceEmitted(blob); emitted > base {
			base = emitted
		}
	}
	return base
}

func randomBlobKnown(blobs [][]byte, blob []byte) bool {
	for _, existing := range blobs {
		if bytes.Equal(existing, blob) {
			return true
		}
	}
	return false
}

func appendRandomBlob(blobs [][]byte, blob []byte) [][]byte {
	if randomBlobKnown(blobs, blob) {
		return blobs
	}
	out := append(append([][]byte(nil), blobs...), append([]byte(nil), blob...))
	if len(out) > maxRandomBlobs {
		out = out[len(out)-maxRandomBlobs:]
	}
	return out
}

type announcePathInput struct {
	destinationKnown bool
	announceHops     uint8
	randomBlob       []byte
	now              time.Time
	// announceAffinity and currentAffinity are effective gravity values
	// (RNS 1.4.1 gravity contests). Go uses pathingAffinity so live iface
	// penalties can break ties without changing configured gravity.
	announceAffinity int
	currentAffinity  int
	affinityKnown    bool
}

func shouldUpdateAnnouncePath(existing *common.Path, in announcePathInput, pathUnresponsive bool) bool {
	add, _ := announcePathDecision(existing, in, pathUnresponsive)
	return add
}

// announcePathDecision resolves the announce path contest. insertionOnly is
// true when the announce wins solely to replace an unresponsive path on the
// same emission timebase (Python table_insertion_only): the path updates but
// the announce is not retransmitted.
func announcePathDecision(existing *common.Path, in announcePathInput, pathUnresponsive bool) (add, insertionOnly bool) {
	if !in.destinationKnown || existing == nil {
		return true, false
	}
	if in.announceHops <= existing.HopCount {
		pathTimebase := timebaseFromRandomBlobs(existing.RandomBlobs)
		emitted := announceEmitted(in.randomBlob)
		if !randomBlobKnown(existing.RandomBlobs, in.randomBlob) && emitted > pathTimebase {
			return true, false
		}
		// Same emission timebase on a higher-affinity interface wins the path
		// (Python: announce_emitted == path_timebase and announce_gravity > current).
		if in.affinityKnown && emitted == pathTimebase && in.announceAffinity > in.currentAffinity {
			return true, false
		}
		return false, false
	}

	emitted := announceEmitted(in.randomBlob)
	pathEmitted := uint32(0)
	for _, blob := range existing.RandomBlobs {
		v := announceEmitted(blob)
		if v >= emitted {
			pathEmitted = v
			break
		}
		if v > pathEmitted {
			pathEmitted = v
		}
	}

	if !existing.Expires.IsZero() && !in.now.Before(existing.Expires) {
		return !randomBlobKnown(existing.RandomBlobs, in.randomBlob), false
	}
	if emitted > pathEmitted {
		return !randomBlobKnown(existing.RandomBlobs, in.randomBlob), false
	}
	if emitted == pathEmitted && pathUnresponsive {
		return true, true
	}
	return false, false
}
