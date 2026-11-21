// SPDX-License-Identifier: LicenseRef-Reticulum
package opusfile_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opusfile"
)

func TestFillLoops(t *testing.T) {
	clip := &opusfile.Clip{PCM: []int16{1, 2, 3}}
	dst := make([]int16, 8)
	pos := 0
	clip.Fill(dst, &pos)
	want := []int16{1, 2, 3, 1, 2, 3, 1, 2}
	for i := range want {
		if dst[i] != want[i] {
			t.Fatalf("dst %v want %v", dst, want)
		}
	}
	if pos != 2 {
		t.Fatalf("pos %d", pos)
	}
}

func TestFillNil(t *testing.T) {
	dst := []int16{9, 9}
	var clip *opusfile.Clip
	pos := 0
	clip.Fill(dst, &pos)
	if dst[0] != 0 || dst[1] != 0 {
		t.Fatalf("dst %v", dst)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	if _, err := opusfile.Decode([]byte("not an ogg file")); err == nil {
		t.Fatal("expected error")
	}
	if _, err := opusfile.Load(""); err == nil {
		t.Fatal("empty path")
	}
}
