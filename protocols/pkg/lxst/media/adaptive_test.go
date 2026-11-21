// SPDX-License-Identifier: LicenseRef-Reticulum
package media_test

import (
	"testing"

	"github.com/Quad4-Software/pbt/pkg/pbt"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/media"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestAdaptiveControllerBitrateClamped(t *testing.T) {
	pbt.Check(t, pbt.ForAll("bitrate stays in proto range", pbt.IntRange(0, 5), func(n int) bool {
		ac := media.NewAdaptiveController()
		for i := 0; i < n+1; i++ {
			ac.Update(media.LinkMetrics{RTT: 5, LossRate: 0.9, JitterMs: 400})
		}
		br := ac.Bitrate()
		return br >= proto.MinBitrate && br <= proto.MaxBitrate
	}), pbt.WithRuns(40))
}
