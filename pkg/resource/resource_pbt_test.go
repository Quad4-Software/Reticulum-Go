// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package resource

import (
	"bytes"
	"testing"

	"github.com/Quad4-Software/pbt/pkg/pbt"
)

func TestPBTResourceSegmentsReassemble(t *testing.T) {
	raw := pbt.Map(
		"payload",
		pbt.SliceOf(pbt.IntRange(0, 255), 0, DefaultSegmentSize*4),
		func(xs []int) []byte {
			b := make([]byte, len(xs))
			for i, v := range xs {
				b[i] = byte(v)
			}
			return b
		},
	)
	prop := pbt.ForAll(
		"concatenated segments equal original bytes",
		raw,
		func(data []byte) bool {
			res, err := New(data, false)
			if err != nil {
				panic(err)
			}
			var buf bytes.Buffer
			for i := uint16(0); i < res.GetSegments(); i++ {
				seg, err := res.GetSegmentData(i)
				if err != nil {
					return false
				}
				buf.Write(seg)
			}
			return bytes.Equal(buf.Bytes(), data)
		},
		pbt.WithShrinker(pbt.SliceShrinker[byte]()),
	)
	pbt.Check(t, prop, pbt.WithRuns(80), pbt.WithSeed(17))
}
