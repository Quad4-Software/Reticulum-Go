// SPDX-License-Identifier: LicenseRef-Reticulum
package tone_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/tone"
)

func TestFillNotSilent(t *testing.T) {
	s := tone.New(tone.DialHz, 48000, 0.2)
	buf := make([]int16, 480)
	s.Fill(buf)
	var peak int16
	for _, v := range buf {
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
	}
	if peak == 0 {
		t.Fatal("silent tone")
	}
}

func TestMute(t *testing.T) {
	s := tone.New(tone.DialHz, 48000, 0.2)
	s.Mute(true)
	buf := make([]int16, 32)
	s.Fill(buf)
	for _, v := range buf {
		if v != 0 {
			t.Fatal("muted tone")
		}
	}
}

func TestDialCadence(t *testing.T) {
	if tone.DialOn(0, 48000) {
		t.Fatal("dial mute at start")
	}
	if !tone.DialOn(48000/10, 48000) {
		t.Fatal("dial on at 100ms")
	}
	if tone.DialOn(48000*3, 48000) {
		t.Fatal("dial off at 3s")
	}
}

func TestBusyCadence(t *testing.T) {
	if tone.BusyOn(0, 48000) {
		t.Fatal("busy starts muted")
	}
	if !tone.BusyOn(48000/4, 48000) {
		t.Fatal("busy on in second half of 500ms")
	}
}
