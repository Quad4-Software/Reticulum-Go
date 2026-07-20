// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/protect"
	"quad4/reticulum-go/pkg/resource"
)

func TestBeginIncomingResourceProtectPrevent(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:         protect.ModePrevent,
		MaxResources: 1,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
	})
	protect.SetDefault(e)

	d1, r1 := protect.AdmitResource(100)
	if !d1.Allow {
		t.Fatal("first resource should allow")
	}
	d2, r2 := protect.AdmitResource(100)
	if d2.Allow {
		t.Fatal("second resource should block under prevent")
	}
	if e.TripCount(protect.ReasonResource) == 0 {
		t.Fatal("expected resource trip")
	}
	r1()
	r2()

	// Smoke that a real begin path still works when under cap.
	l := &Link{mdu: 500}
	adv := &resource.ResourceAdvertisement{
		Parts:        1,
		TransferSize: 32,
		Hashmap:      make([]byte, resource.MapHashLen),
		Hash:         make([]byte, 32),
		RandomHash:   make([]byte, resource.RandomHashSize),
	}
	if err := l.beginIncomingResource(adv); err != nil {
		t.Fatalf("beginIncomingResource: %v", err)
	}
	l.resetIncomingResource()
}
