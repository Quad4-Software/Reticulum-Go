// SPDX-License-Identifier: LicenseRef-Reticulum
package filter_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/filter"
)

func TestBandPassSilence(t *testing.T) {
	bp := filter.NewBandPass(250, 8500, 48000)
	pcm := make([]int16, 64)
	bp.Process(pcm)
	for _, s := range pcm {
		if s != 0 {
			t.Fatalf("silence became %d", s)
		}
	}
}

func TestAGCRaisesQuiet(t *testing.T) {
	agc := filter.NewAGC(-15)
	pcm := make([]int16, 128)
	for i := range pcm {
		pcm[i] = 200
	}
	agc.Process(pcm)
	if pcm[0] <= 200 {
		t.Fatalf("gain not applied %d", pcm[0])
	}
}

func TestEchoSuppressor(t *testing.T) {
	e := filter.NewEchoSuppressor()
	ref := make([]int16, 32)
	near := make([]int16, 32)
	for i := range ref {
		ref[i] = 20000
		near[i] = 1000
	}
	e.SetReference(ref)
	e.Process(near)
	if near[0] >= 1000 {
		t.Fatalf("expected attenuation %d", near[0])
	}
}

func TestApplyGain(t *testing.T) {
	pcm := []int16{1000}
	filter.ApplyGain(pcm, 6)
	if pcm[0] <= 1000 {
		t.Fatalf("gain %d", pcm[0])
	}
}
