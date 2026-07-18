// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"math"
	"testing"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/pbt/pkg/pbt"
)

func TestParseRequestedAtRejectsNonFinite(t *testing.T) {
	for _, v := range []any{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := parseRequestedAt(v); err == nil {
			t.Fatalf("expected error for %v", v)
		}
	}
}

func TestPBTRequestTimestampSkewWindow(t *testing.T) {
	delta := pbt.IntRange(-RequestTimestampMaxSkewPast-30, RequestTimestampMaxSkewFuture+30)
	prop := pbt.ForAll(
		"request timestamp skew window",
		delta,
		func(sec int) bool {
			now := time.Unix(1_700_000_000, 0)
			at := now.Add(time.Duration(sec) * time.Second)
			ok := requestTimestampValid(at, now)
			inWindow := sec >= -RequestTimestampMaxSkewPast && sec <= RequestTimestampMaxSkewFuture
			return ok == inWindow
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(80), pbt.WithSeed(88))
}

func FuzzParseRequestedAtMsgpack(f *testing.F) {
	now := float64(time.Now().Unix())
	b, _ := msgpack.Marshal(now)
	f.Add(b)
	b2, _ := msgpack.Marshal(int64(time.Now().Unix()))
	f.Add(b2)
	f.Add([]byte{})
	f.Add([]byte{0xc0})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64 {
			t.Skip()
		}
		var v any
		if err := msgpack.Unmarshal(data, &v); err != nil {
			return
		}
		at, err := parseRequestedAt(v)
		if err != nil {
			return
		}
		if at.IsZero() {
			t.Fatal("zero time without error")
		}
	})
}
