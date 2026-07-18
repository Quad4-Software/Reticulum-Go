// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package announce

import (
	"strings"
	"testing"

	"quad4/pbt/pkg/pbt"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
)

func TestPBTCreateThenHandleAnnounce(t *testing.T) {
	appData := pbt.Map(
		"appData",
		pbt.SliceOf(pbt.IntRange(0, 255), 0, 48),
		func(xs []int) []byte {
			b := make([]byte, len(xs))
			for i, v := range xs {
				b[i] = byte(v)
			}
			return b
		},
	)
	prop := pbt.ForAll(
		"create packet then handle succeeds",
		appData,
		func(data []byte) bool {
			id, err := identity.New()
			if err != nil {
				return false
			}
			ann, err := New(id, make([]byte, 16), "pbtapp", data, false, &common.ReticulumConfig{})
			if err != nil {
				return false
			}
			pkt, err := ann.CreatePacket()
			if err != nil {
				return false
			}
			return ann.HandleAnnounce(pkt) == nil
		},
		pbt.WithShrinker(pbt.SliceShrinker[byte]()),
	)
	pbt.Check(t, prop, pbt.WithRuns(40), pbt.WithSeed(31))
}

func TestPBTTruncatedAnnounceAlwaysErrors(t *testing.T) {
	length := pbt.IntRange(0, MinAnnouncePacketSizeNoRatchet-1)
	prop := pbt.ForAll(
		"truncated announce always errors",
		length,
		func(n int) bool {
			id, err := identity.New()
			if err != nil {
				return false
			}
			ann, err := New(id, make([]byte, 16), "pbttrunc", nil, false, &common.ReticulumConfig{})
			if err != nil {
				return false
			}
			buf := make([]byte, n)
			if n > 0 {
				buf[0] = PacketTypeAnnounce
			}
			err = ann.HandleAnnounce(buf)
			return err != nil
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(60), pbt.WithSeed(32))
}

func TestPBTHopOverflowAlwaysErrors(t *testing.T) {
	hops := pbt.IntRange(int(MaxHops)+1, 255)
	prop := pbt.ForAll(
		"hop overflow always errors",
		hops,
		func(h int) bool {
			id, err := identity.New()
			if err != nil {
				return false
			}
			ann, err := New(id, make([]byte, 16), "pbthops", nil, false, &common.ReticulumConfig{})
			if err != nil {
				return false
			}
			buf := make([]byte, MinAnnouncePacketSizeNoRatchet)
			buf[0] = PacketTypeAnnounce
			buf[1] = byte(h)
			err = ann.HandleAnnounce(buf)
			return err != nil && strings.Contains(err.Error(), "hop")
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(40), pbt.WithSeed(33))
}
