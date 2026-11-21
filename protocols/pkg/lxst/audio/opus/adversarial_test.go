// SPDX-License-Identifier: LicenseRef-Reticulum
package opus_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opus"
)

func TestDecodeEmptyRejected(t *testing.T) {
	dec, err := opus.NewDecoder(opus.DefaultSampleRate, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dec.Close() }()
	if _, err := dec.Decode(nil); err == nil {
		t.Fatal("empty packet")
	}
}

func TestDecodeOwnsNextCall(t *testing.T) {
	enc, err := opus.NewEncoder(opus.DefaultSampleRate, 1, 16000)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = enc.Close() }()
	dec, err := opus.NewDecoder(opus.DefaultSampleRate, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dec.Close() }()
	a := make([]int16, opus.DefaultFrameSize)
	b := make([]int16, opus.DefaultFrameSize)
	for i := range a {
		a[i] = 200
		b[i] = -200
	}
	pa, err := enc.Encode(a)
	if err != nil {
		t.Fatal(err)
	}
	pa = append([]byte(nil), pa...)
	pb, err := enc.Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	pb = append([]byte(nil), pb...)
	out1, err := dec.Decode(append([]byte(nil), pa...))
	if err != nil {
		t.Fatal(err)
	}
	first := out1[0]
	owned := append([]int16(nil), out1...)
	out2, err := dec.Decode(append([]byte(nil), pb...))
	if err != nil {
		t.Fatal(err)
	}
	if len(out2) == 0 {
		t.Fatal("empty second decode")
	}
	if owned[0] != first {
		t.Fatal("caller copy of first decode was mutated")
	}
}

func TestClosedDecoder(t *testing.T) {
	dec, err := opus.NewDecoder(opus.DefaultSampleRate, 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = dec.Close()
	if _, err := dec.Decode([]byte{1, 2, 3}); err == nil {
		t.Fatal("decode after close")
	}
}

func FuzzDownsampleLen(f *testing.F) {
	f.Add(48)
	f.Fuzz(func(t *testing.T, n int) {
		if n < 0 {
			n = -n
		}
		n = n % 4800
		pcm := make([]int16, n)
		out := opus.Downsample(pcm, 48000, 8000)
		if 48000%8000 == 0 {
			want := n / (48000 / 8000)
			if len(out) != want {
				t.Fatalf("len %d want %d", len(out), want)
			}
		}
	})
}
