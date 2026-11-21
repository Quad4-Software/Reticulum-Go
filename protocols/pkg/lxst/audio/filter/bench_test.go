// SPDX-License-Identifier: LicenseRef-Reticulum
package filter_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/filter"
)

func BenchmarkBandPass(b *testing.B) {
	bp := filter.NewBandPass(250, 8500, 48000)
	pcm := make([]int16, 960)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bp.Process(pcm)
	}
}

func TestBandPassNoAlloc(t *testing.T) {
	bp := filter.NewBandPass(250, 8500, 48000)
	pcm := make([]int16, 960)
	n := testing.AllocsPerRun(200, func() {
		bp.Process(pcm)
	})
	if n != 0 {
		t.Fatalf("process allocs %v", n)
	}
}
