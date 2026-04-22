// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package rate

import (
	"testing"

	"git.quad4.io/Go-Libs/pbt/pkg/pbt"
)

func TestPBTAnnounceRateControlDisabledAlwaysAllows(t *testing.T) {
	arc := NewAnnounceRateControl(0, 0, 0)
	prop := pbt.ForAll(
		"zero config allows every announce key",
		pbt.StringASCII(0, 64),
		func(hash string) bool {
			return arc.AllowAnnounce(hash)
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(80), pbt.WithSeed(88))
}
