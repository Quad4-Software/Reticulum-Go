// SPDX-License-Identifier: LicenseRef-Reticulum
package opus_test

import (
	"math/rand"
	"testing"

	"github.com/Quad4-Software/pbt/pkg/pbt"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opus"
)

func TestEncodeNeverEmptyOnPCM(t *testing.T) {
	enc, err := opus.NewEncoder(opus.DefaultSampleRate, 1, 16000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = enc.Close() })
	gen := pbt.NewGenerator("pcm", func(r *rand.Rand, _ int) []int16 {
		pcm := make([]int16, opus.DefaultFrameSize)
		for i := range pcm {
			pcm[i] = int16(r.Intn(600) - 300)
		}
		return pcm
	})
	pbt.Check(t, pbt.ForAll("opus encode yields bytes", gen, func(pcm []int16) bool {
		out, err := enc.Encode(pcm)
		return err == nil && len(out) > 0
	}), pbt.WithRuns(30))
}

func TestEncodeRejectsShortFrame(t *testing.T) {
	enc, err := opus.NewEncoder(opus.DefaultSampleRate, 1, 16000)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = enc.Close() }()
	if _, err := enc.Encode([]int16{1, 2, 3}); err == nil {
		t.Fatal("short pcm")
	}
}

func TestClosedEncoder(t *testing.T) {
	enc, err := opus.NewEncoder(opus.DefaultSampleRate, 1, 16000)
	if err != nil {
		t.Fatal(err)
	}
	_ = enc.Close()
	pcm := make([]int16, opus.DefaultFrameSize)
	if _, err := enc.Encode(pcm); err == nil {
		t.Fatal("encode after close")
	}
}
