// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package buffer

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/Quad4-Software/pbt/pkg/pbt"
)

func genStreamDataMessage(r *rand.Rand, size int) StreamDataMessage {
	maxData := 8192
	if size > 0 && size*80 < maxData {
		maxData = size * 80
	}
	if maxData < 0 {
		maxData = 0
	}
	n := r.Intn(maxData + 1)
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(r.Intn(256))
	}
	return StreamDataMessage{
		StreamID:   uint16(r.Intn(0x4000)),
		Data:       data,
		EOF:        r.Intn(2) == 1,
		Compressed: r.Intn(2) == 1,
	}
}

func TestPBTStreamDataMessageRoundTrip(t *testing.T) {
	gen := pbt.NewGenerator("streamData", genStreamDataMessage)
	prop := pbt.ForAll(
		"pack unpack preserves stream fields",
		gen,
		func(orig StreamDataMessage) bool {
			raw, err := orig.Pack()
			if err != nil {
				return false
			}
			var got StreamDataMessage
			if err := got.Unpack(raw); err != nil {
				return false
			}
			wantID := orig.StreamID & StreamIDMax
			return got.StreamID == wantID && got.EOF == orig.EOF && got.Compressed == orig.Compressed &&
				bytes.Equal(got.Data, orig.Data)
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(100), pbt.WithSeed(5), pbt.WithMaxSize(120))
}
