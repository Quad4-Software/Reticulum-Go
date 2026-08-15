// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/protect"
)

func TestProcessIncomingProtectPreventDrops(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := protect.New(protect.Options{
		Mode:            protect.ModePrevent,
		MaxPPS:          3,
		WarnWriter:      &buf,
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		DisableCoolDown: true,
		Now:             func() time.Time { return clock },
	})
	protect.SetDefault(e)

	base := NewBaseInterface("mock0", common.IFTypeUDP, true)
	var calls atomic.Int64
	base.SetPacketCallback(func(data []byte, iface common.NetworkInterface) {
		calls.Add(1)
	})
	pkt := []byte{0x00, 0x00, 0x01, 0x02}
	for range 20 {
		base.ProcessIncoming(pkt)
	}
	if calls.Load() == 0 {
		t.Fatal("expected some callbacks before threshold")
	}
	if calls.Load() >= 20 {
		t.Fatalf("prevent should drop some got %d", calls.Load())
	}
	if e.TripCount(protect.ReasonPPS) == 0 {
		t.Fatal("expected pps trips")
	}
}

func TestProcessIncomingProtectDetectAllows(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := protect.New(protect.Options{
		Mode:            protect.ModeDetect,
		MaxPPS:          3,
		WarnWriter:      &buf,
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		DisableCoolDown: true,
		Now:             func() time.Time { return clock },
	})
	protect.SetDefault(e)

	base := NewBaseInterface("mock1", common.IFTypeUDP, true)
	var calls atomic.Int64
	base.SetPacketCallback(func(data []byte, iface common.NetworkInterface) {
		calls.Add(1)
	})
	pkt := []byte{0x00, 0x00}
	for range 20 {
		base.ProcessIncoming(pkt)
	}
	if calls.Load() != 20 {
		t.Fatalf("detect must deliver all got %d", calls.Load())
	}
	if e.TripCount(protect.ReasonPPS) == 0 {
		t.Fatal("expected trips")
	}
}
