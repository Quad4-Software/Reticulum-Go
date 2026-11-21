// SPDX-License-Identifier: LicenseRef-Reticulum
package io_test

import (
	"sync"
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/io"
)

func TestRaceNullDevice(t *testing.T) {
	dev := io.NewNullDevice()
	_ = dev.Start()
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for range 200 {
			_, _ = dev.ReadPCM()
		}
	}()
	go func() {
		defer wg.Done()
		pcm := make([]int16, io.DefaultFrameSize)
		for range 200 {
			_ = dev.WritePCM(pcm)
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(time.Millisecond)
		_ = dev.Close()
	}()
	wg.Wait()
}

func TestNullDevice(t *testing.T) {
	dev := io.NewNullDevice()
	if err := dev.Start(); err != nil {
		t.Fatal(err)
	}
	pcm, err := dev.ReadPCM()
	if err != nil {
		t.Fatal(err)
	}
	if len(pcm) != io.DefaultFrameSize {
		t.Fatalf("unexpected frame size: %d", len(pcm))
	}
	if err := dev.WritePCM(pcm); err != nil {
		t.Fatal(err)
	}
	if err := dev.Close(); err != nil {
		t.Fatal(err)
	}
	if err := dev.Start(); err != io.ErrDeviceClosed {
		t.Fatalf("start after close: %v", err)
	}
	if _, err := dev.ReadPCM(); err != io.ErrDeviceClosed {
		t.Fatalf("read after close: %v", err)
	}
	if err := dev.WritePCM(pcm); err != io.ErrDeviceClosed {
		t.Fatalf("write after close: %v", err)
	}
}

func TestNewDeviceFallback(t *testing.T) {
	dev, err := io.NewDevice(true)
	if err != nil {
		t.Skip(err.Error())
	}
	if dev == nil {
		t.Fatal("nil device")
	}
	if err := dev.Start(); err != nil {
		t.Fatal(err)
	}
	pcm := make([]int16, io.DefaultFrameSize)
	if err := dev.WritePCM(pcm); err != nil {
		t.Fatal(err)
	}
	if _, err := dev.ReadPCM(); err != nil {
		t.Fatal(err)
	}
	_ = dev.Close()
}

func TestOpenPlayback(t *testing.T) {
	dev, err := io.Open(io.Options{Role: io.RolePlayback})
	if err != nil {
		t.Skip(err.Error())
	}
	if dev == nil {
		t.Fatal("nil device")
	}
	if err := dev.Start(); err != nil {
		t.Fatal(err)
	}
	pcm := make([]int16, io.DefaultFrameSize)
	if err := dev.WritePCM(pcm); err != nil {
		t.Fatal(err)
	}
	_ = dev.Close()
}

func TestListDevices(t *testing.T) {
	_, err := io.ListDevices()
	if err != nil {
		t.Fatal(err)
	}
}

func TestBackend(t *testing.T) {
	switch io.Backend() {
	case "miniaudio", "null":
	default:
		t.Fatalf("backend %q", io.Backend())
	}
}
