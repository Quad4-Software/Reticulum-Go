//go:build cgo && lxst_native

// SPDX-License-Identifier: LicenseRef-Reticulum
package opusfile_test

import (
	"path/filepath"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opusfile"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/sounds"
)

func TestDecodeRinger(t *testing.T) {
	b, err := sounds.FS.ReadFile("ringer.opus")
	if err != nil {
		t.Fatal(err)
	}
	clip, err := opusfile.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(clip.PCM) < 4800 {
		t.Fatalf("short pcm %d", len(clip.PCM))
	}
	dst := make([]int16, 960)
	pos := 0
	clip.Fill(dst, &pos)
	var energy int64
	for _, s := range dst {
		energy += int64(s) * int64(s)
	}
	if energy == 0 {
		t.Fatal("silent ringtone frame")
	}
}

func TestDecodeSoft(t *testing.T) {
	b, err := sounds.FS.ReadFile("soft.opus")
	if err != nil {
		t.Fatal(err)
	}
	clip, err := opusfile.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(clip.PCM) < 4800 {
		t.Fatalf("short pcm %d", len(clip.PCM))
	}
}

func TestLoadRingerFile(t *testing.T) {
	dir := t.TempDir()
	if err := sounds.Install(dir); err != nil {
		t.Fatal(err)
	}
	clip, err := opusfile.Load(filepath.Join(dir, "ringer.opus"))
	if err != nil {
		t.Fatal(err)
	}
	if len(clip.PCM) < 4800 {
		t.Fatalf("short pcm %d", len(clip.PCM))
	}
}
