// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package destination

import (
	"context"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/stamps"
)

func TestDestinationValueClamping(t *testing.T) {
	cfg := &common.ReticulumConfig{Stamps: common.StampsConfig{DestinationValue: 25, DestinationValueSet: true}}
	tr := &mockTransport{config: cfg}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(id, In, Single, "app", tr)
	if err != nil {
		t.Fatal(err)
	}

	// Default: target value is DESTINATION_VALUE.
	if v := d.TargetValue(); v != 25 {
		t.Fatalf("TargetValue = %v, want 25", v)
	}
	// Setting below the floor clamps up.
	d.SetValue(10)
	if v := d.TargetValue(); v != 25 {
		t.Fatalf("TargetValue = %v, want 25 (floor)", v)
	}
	// Above the cap clamps down.
	d.SetValue(500)
	if v := d.TargetValue(); v != common.StampCostCap {
		t.Fatalf("TargetValue = %v, want %d", v, common.StampCostCap)
	}
	d.SetValue(30)
	if v := d.TargetValue(); v != 30 {
		t.Fatalf("TargetValue = %v, want 30", v)
	}
}

func TestDestinationLinkCost(t *testing.T) {
	tr := &mockTransport{config: &common.ReticulumConfig{}}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(id, In, Single, "app", tr)
	if err != nil {
		t.Fatal(err)
	}

	// Default is transport LR_COST (12.5).
	if c := d.GetLinkCost(); c != 12.5 {
		t.Fatalf("GetLinkCost = %v, want 12.5", c)
	}
	// Config override applies when no per-destination cost is set.
	tr.config = &common.ReticulumConfig{Stamps: common.StampsConfig{LRCost: 33, LRCostSet: true}}
	if c := d.GetLinkCost(); c != 33 {
		t.Fatalf("GetLinkCost = %v, want 33", c)
	}
	// Per-destination cost wins and clamps to LINK_COST_CAP.
	d.SetLinkCost(99)
	if c := d.GetLinkCost(); c != common.LinkCostCap {
		t.Fatalf("GetLinkCost = %v, want %d", c, common.LinkCostCap)
	}
	d.SetLinkCost(7.5)
	if c := d.GetLinkCost(); c != 7.5 {
		t.Fatalf("GetLinkCost = %v, want 7.5", c)
	}
	d.SetLinkCost(-4)
	if c := d.GetLinkCost(); c != 0 {
		t.Fatalf("GetLinkCost = %v, want 0", c)
	}
}

func TestDestinationAwaitStampGeneratesAndPersists(t *testing.T) {
	storageDir := t.TempDir()
	t.Setenv("RETICULUM_STORAGE_PATH", storageDir)

	cfg := &common.ReticulumConfig{Stamps: common.StampsConfig{
		DestinationValue:    8,
		DestinationValueSet: true,
	}}
	tr := &mockTransport{config: cfg}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(id, In, Single, "app", tr)
	if err != nil {
		t.Fatal(err)
	}
	settings := cfg.StampsSettings()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if !d.awaitStamp(ctx, settings) {
		t.Fatal("awaitStamp failed")
	}
	stamp, value := d.Stamp()
	if len(stamp) != stamps.StampLength {
		t.Fatalf("stamp len=%d", len(stamp))
	}
	if value < 8 {
		t.Fatalf("stamp value %v below target 8", value)
	}
	if !stamps.StampValid(stamp, 8, d.stampMaterial()) {
		t.Fatal("stamp does not validate against material")
	}
	// Persisted file exists and reloads.
	got, gotValue, ok := stamps.LoadDestinationStamp(storageDir, d.hashValue, d.stampMaterial(), 8)
	if !ok || string(got) != string(stamp) || gotValue != value {
		t.Fatalf("persisted stamp mismatch ok=%v", ok)
	}
}

func TestDestinationStampMaterial(t *testing.T) {
	tr := &mockTransport{config: &common.ReticulumConfig{}}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(id, In, Single, "app", tr, "aspect")
	if err != nil {
		t.Fatal(err)
	}
	m := d.stampMaterial()
	if len(m) != 16+64+10 {
		t.Fatalf("material len=%d", len(m))
	}
}

func TestStampedAnnounceEmit(t *testing.T) {
	storageDir := t.TempDir()
	t.Setenv("RETICULUM_STORAGE_PATH", storageDir)

	cfg := &common.ReticulumConfig{Stamps: common.StampsConfig{
		StampDestinations:   true,
		DestinationValue:    8,
		DestinationValueSet: true,
	}}
	iface := newRecordingInterface("out0")
	tr := &mockTransport{config: cfg, interfaces: map[string]common.NetworkInterface{"out0": iface}}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(id, In, Single, "app", tr)
	if err != nil {
		t.Fatal(err)
	}
	// Pre-stamp so Announce dispatches immediately.
	if !d.awaitStamp(context.Background(), cfg.StampsSettings()) {
		t.Fatal("awaitStamp failed")
	}
	d.SetLinkCost(9.5)
	if err := d.Announce(false, nil, nil); err != nil {
		t.Fatalf("Announce: %v", err)
	}
	sent := iface.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent=%d", len(sent))
	}
	pkt := sent[0]
	ext := pkt[len(pkt)-stamps.ExtPayloadLen:]
	if ext[0] != stamps.ExtPayloadMarker || ext[1] != 0x00 {
		t.Fatalf("ext header %x", ext[:2])
	}
	stamp, _ := d.Stamp()
	if string(ext[2:16]) != string(stamp) {
		t.Fatal("ext stamp mismatch")
	}
	// lr_cost bytes are [frac, int].
	if ext[16] != byte(int(9.5*256)%256) || ext[17] != 9 {
		t.Fatalf("lr cost bytes %x", ext[16:])
	}
	// Signature still validates over the stamped app_data via the receive path.
}

func TestStampedAnnounceDeferred(t *testing.T) {
	t.Setenv("RETICULUM_STORAGE_PATH", t.TempDir())
	StopStamper()
	t.Cleanup(StopStamper)

	cfg := &common.ReticulumConfig{Stamps: common.StampsConfig{
		StampDestinations:   true,
		DestinationValue:    8,
		DestinationValueSet: true,
	}}
	iface := newRecordingInterface("out0")
	tr := &mockTransport{config: cfg, interfaces: map[string]common.NetworkInterface{"out0": iface}}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(id, In, Single, "app", tr)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Announce(false, nil, nil); err != nil {
		t.Fatalf("Announce: %v", err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if len(iface.Sent()) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	sent := iface.Sent()
	if len(sent) == 0 {
		t.Fatal("background stamper never emitted the announce")
	}
	pkt := sent[0]
	if pkt[len(pkt)-stamps.ExtPayloadLen] != stamps.ExtPayloadMarker {
		t.Fatal("emitted announce missing stamp ext")
	}
}
