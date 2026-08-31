// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package profiler

import (
	"encoding/binary"
	"testing"
)

// FuzzPrettyShortTime ensures formatting never panics on arbitrary floats.
func FuzzPrettyShortTime(f *testing.F) {
	f.Add(float64(0))
	f.Add(float64(1e-9))
	f.Add(float64(1e-3))
	f.Add(float64(1))
	f.Add(float64(60))
	f.Add(float64(-1))
	f.Fuzz(func(t *testing.T, sec float64) {
		_ = prettyShortTime(sec)
	})
}

// FuzzCalcStats exercises summary stats on arbitrary duration sequences.
func FuzzCalcStats(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0})
	f.Add([]byte{0, 0, 128, 63, 0, 0, 0, 64}) // 1.0, 2.0 little-endian float32-ish
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) < 4 {
			return
		}
		n := int(raw[0])%64 + 1
		caps := make([]capture, 0, n)
		base := float64(1_000_000)
		for i := 0; i < n && 1+i*4+3 < len(raw); i++ {
			d := float64(binary.LittleEndian.Uint32(raw[1+i*4:1+i*4+4])) / 1e9
			caps = append(caps, capture{
				start:       base + float64(i),
				end:         base + float64(i) + d,
				threadIdent: uint64(i%7 + 1),
				done:        true,
			})
		}
		_ = calcStats(caps, 0, len(caps), true)
		_ = FormatResults(map[string]TagResult{
			"fuzz": {Name: "fuzz", StatsAll: calcStats(caps, 0, len(caps), true)},
		})
	})
}
