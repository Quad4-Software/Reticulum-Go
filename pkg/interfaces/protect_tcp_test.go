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

func TestTCPServerProtectConnCap(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:         protect.ModePrevent,
		MaxConns:     2,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
	})
	protect.SetDefault(e)

	d1, r1 := protect.AdmitConn("tcp-protect")
	d2, r2 := protect.AdmitConn("tcp-protect")
	d3, r3 := protect.AdmitConn("tcp-protect")
	if !d1.Allow || !d2.Allow || d3.Allow {
		t.Fatalf("conn cap d1=%v d2=%v d3=%v", d1.Allow, d2.Allow, d3.Allow)
	}
	if e.TripCount(protect.ReasonConn) == 0 {
		t.Fatal("expected conn trip")
	}
	r1()
	r2()
	r3()
	d4, r4 := protect.AdmitConn("tcp-protect")
	if !d4.Allow {
		t.Fatal("after release should allow")
	}
	r4()
}

func TestIfaceChaosProtectFlood(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := protect.New(protect.Options{
		Mode:            protect.ModePrevent,
		MaxPPS:          20,
		WarnWriter:      &buf,
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		DisableCoolDown: true,
		Now:             func() time.Time { return clock },
	})
	protect.SetDefault(e)

	base := NewBaseInterface("chaos0", common.IFTypeUDP, true)
	var calls atomic.Int64
	base.SetPacketCallback(func(data []byte, iface common.NetworkInterface) {
		calls.Add(1)
	})
	pkt := []byte{0x00, 0x00}
	for range 200 {
		base.ProcessIncoming(pkt)
	}
	if calls.Load() >= 200 {
		t.Fatalf("chaos prevent flood should shed got %d", calls.Load())
	}
	if e.TripCount(protect.ReasonPPS) == 0 {
		t.Fatal("expected pps trips")
	}
}
