// SPDX-License-Identifier: LicenseRef-Reticulum
package io_test

import (
	"bytes"
	"sync"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/io"
)

func TestPCM16LERoundTrip(t *testing.T) {
	in := []int16{-32768, -1, 0, 1, 32767}
	raw := io.PCM16LE(in)
	out, err := io.FromPCM16LE(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(io.PCM16LE(out), raw) {
		t.Fatalf("got %v want %v", out, in)
	}
}

func TestFromPCM16LERejectsOdd(t *testing.T) {
	if _, err := io.FromPCM16LE([]byte{1}); err == nil {
		t.Fatal("expected error")
	}
}

func TestFromPCM16LERejectsHuge(t *testing.T) {
	if _, err := io.FromPCM16LE(make([]byte, io.MaxPCMBytes+2)); err == nil {
		t.Fatal("expected error")
	}
}

func TestHostPushPull(t *testing.T) {
	h := io.NewHost()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	pcm := []int16{1, 2, 3, 4}
	if err := h.Push(pcm); err != nil {
		t.Fatal(err)
	}
	got, err := h.ReadPCM()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Fatalf("read %v", got)
	}
	if err := h.WritePCM([]int16{9, 8}); err != nil {
		t.Fatal(err)
	}
	play, ok := h.Pull()
	if !ok || len(play) != 2 || play[0] != 9 {
		t.Fatalf("pull %v %v", play, ok)
	}
	if _, ok := h.Pull(); ok {
		t.Fatal("empty pull")
	}
}

func TestHostReadSilenceWhenEmpty(t *testing.T) {
	h := io.NewHost()
	got, err := h.ReadPCM()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != io.DefaultFrameSize {
		t.Fatalf("silence %d", len(got))
	}
	for _, s := range got {
		if s != 0 {
			t.Fatal("expected silence")
		}
	}
}

func TestHostPlaybackHandler(t *testing.T) {
	h := io.NewHost()
	got := make(chan []int16, 1)
	h.SetPlaybackHandler(func(pcm []int16) { got <- append([]int16(nil), pcm...) })
	if err := h.WritePCM([]int16{5, 6}); err != nil {
		t.Fatal(err)
	}
	pcm := <-got
	if len(pcm) != 2 || pcm[0] != 5 {
		t.Fatalf("handler %v", pcm)
	}
}

func TestHostClose(t *testing.T) {
	h := io.NewHost()
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if err := h.Push([]int16{1}); err != io.ErrDeviceClosed {
		t.Fatalf("push: %v", err)
	}
	if _, err := h.ReadPCM(); err != io.ErrDeviceClosed {
		t.Fatalf("read: %v", err)
	}
	if err := h.Start(); err != io.ErrDeviceClosed {
		t.Fatalf("start: %v", err)
	}
}

func TestHostDropOldest(t *testing.T) {
	h := io.NewHost()
	for i := range 40 {
		if err := h.Push([]int16{int16(i)}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := h.ReadPCM()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 8 {
		t.Fatalf("oldest dropped, got %v", got)
	}
	st := h.Stats()
	if st.CaptureDropped != 8 {
		t.Fatalf("dropped %d", st.CaptureDropped)
	}
}

func TestHostBytes(t *testing.T) {
	h := io.NewHost()
	raw := io.PCM16LE([]int16{11, 12})
	if err := h.PushBytes(raw); err != nil {
		t.Fatal(err)
	}
	got, err := h.ReadPCM()
	if err != nil || len(got) != 2 || got[1] != 12 {
		t.Fatalf("push bytes %v %v", got, err)
	}
	if err := h.WritePCM([]int16{21, 22}); err != nil {
		t.Fatal(err)
	}
	out, ok := h.PullBytes()
	if !ok {
		t.Fatal("pull bytes")
	}
	pcm, err := io.FromPCM16LE(out)
	if err != nil || len(pcm) != 2 || pcm[0] != 21 {
		t.Fatalf("pulled %v %v", pcm, err)
	}
}

func TestRaceHost(t *testing.T) {
	h := io.NewHost()
	_ = h.Start()
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		for range 200 {
			_ = h.Push([]int16{1, 2, 3})
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			_, _ = h.ReadPCM()
		}
	}()
	go func() {
		defer wg.Done()
		pcm := make([]int16, io.DefaultFrameSize)
		for range 200 {
			_ = h.WritePCM(pcm)
			h.Pull()
		}
	}()
	go func() {
		defer wg.Done()
		for range 50 {
			_ = h.PushBytes(io.PCM16LE([]int16{4, 5}))
		}
	}()
	wg.Wait()
	_ = h.Close()
}
