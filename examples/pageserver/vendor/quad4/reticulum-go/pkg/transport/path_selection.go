// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"time"

	"quad4/reticulum-go/pkg/common"
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
}

func shouldUpdateAnnouncePath(existing *common.Path, in announcePathInput, pathUnresponsive bool) bool {
	if !in.destinationKnown || existing == nil {
		return true
	}
	if in.announceHops <= existing.HopCount {
		pathTimebase := timebaseFromRandomBlobs(existing.RandomBlobs)
		emitted := announceEmitted(in.randomBlob)
		if !randomBlobKnown(existing.RandomBlobs, in.randomBlob) && emitted > pathTimebase {
			return true
		}
		return false
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
		return !randomBlobKnown(existing.RandomBlobs, in.randomBlob)
	}
	if emitted > pathEmitted {
		return !randomBlobKnown(existing.RandomBlobs, in.randomBlob)
	}
	if emitted == pathEmitted && pathUnresponsive {
		return true
	}
	return false
}
