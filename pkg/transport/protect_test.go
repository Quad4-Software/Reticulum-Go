// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/protect"
)

func occupyHandlerPool(t *testing.T, tr *Transport) {
	t.Helper()
	hold := make(chan struct{})
	if n := tr.occupyHandlerPoolForTest(hold); n == 0 {
		t.Fatal("handler pool not occupied")
	}
	t.Cleanup(func() { close(hold) })
}

func TestHandlePacketProtectPreventShedsOnSemFull(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:         protect.ModePrevent,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
	})

	cfg := &common.ReticulumConfig{EnableTransport: true, DoSProtection: "off", MaxPacketHandlers: 4}
	tr := NewTransport(cfg)
	protect.SetDefault(e)
	t.Cleanup(func() {
		_ = tr.Close()
		protect.SetDefault(nil)
	})

	occupyHandlerPool(t, tr)

	iface := common.NewBaseInterface("flood0", common.IFTypeUDP, true)
	pkt := []byte{0x00, 0x00, 0x01}
	tr.HandlePacket(pkt, &iface)
	if e.TripCount(protect.ReasonHandler) == 0 {
		t.Fatal("expected handler trip")
	}
	if !bytes.Contains(buf.Bytes(), []byte("reason=handler")) {
		t.Fatalf("warn missing %q", buf.String())
	}
}

func TestHandlePacketProtectDetectShedsOnSemFull(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:         protect.ModeDetect,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
	})
	cfg := &common.ReticulumConfig{EnableTransport: true, DoSProtection: "detect", MaxPacketHandlers: 4}
	tr := NewTransport(cfg)
	protect.SetDefault(e)
	t.Cleanup(func() {
		_ = tr.Close()
		protect.SetDefault(nil)
	})
	occupyHandlerPool(t, tr)
	iface := common.NewBaseInterface("flood1", common.IFTypeUDP, true)
	tr.HandlePacket([]byte{0x00, 0x00, 0x01}, &iface)
	if e.TripCount(protect.ReasonHandler) == 0 {
		t.Fatal("expected handler trip in detect mode")
	}
}

func TestHandlePacketProtectAutoLearningShedsOnSemFull(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	e := protect.New(protect.Options{
		Mode:                 protect.ModeAuto,
		WarnWriter:           &bytes.Buffer{},
		WarnInterval:         time.Hour,
		AutoLearnMinDuration: time.Hour,
	})
	cfg := &common.ReticulumConfig{EnableTransport: true, DoSProtection: "auto", MaxPacketHandlers: 4}
	tr := NewTransport(cfg)
	protect.SetDefault(e)
	t.Cleanup(func() {
		_ = tr.Close()
		protect.SetDefault(nil)
	})
	occupyHandlerPool(t, tr)
	iface := common.NewBaseInterface("flood2", common.IFTypeUDP, true)
	tr.HandlePacket([]byte{0x00, 0x00, 0x01}, &iface)
	if e.TripCount(protect.ReasonHandler) == 0 {
		t.Fatal("expected handler trip while auto learning")
	}
	if e.Phase() != protect.AutoLearning {
		t.Fatalf("phase=%v want learning", e.Phase())
	}
}

func TestAdversarialProtectPacketFloodBudget(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := protect.New(protect.Options{
		Mode:            protect.ModePrevent,
		MaxPPS:          50,
		WarnWriter:      &buf,
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		DisableCoolDown: true,
		Now:             func() time.Time { return clock },
	})
	protect.SetDefault(e)

	iface := common.NewBaseInterface("adv0", common.IFTypeUDP, true)
	// Use interfaces.BaseInterface path via wrapping: ProcessIncoming on common base is stub.
	// Flood via protect admit directly for budget assertion.
	allowed := 0
	blocked := 0
	for range 500 {
		d := e.AdmitPacket(iface.GetName(), 64)
		if d.Allow {
			allowed++
		} else {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatal("expected blocked packets under prevent flood")
	}
	if allowed == 0 {
		t.Fatal("expected some allowed before threshold")
	}
	if e.TripCount(protect.ReasonPPS) == 0 {
		t.Fatal("expected pps trips")
	}
}
