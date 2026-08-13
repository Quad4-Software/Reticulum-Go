// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
)

func TestShouldUpdateAnnouncePathHigherGravity(t *testing.T) {
	blob := []byte{1, 2, 3, 4, 5, 0, 0, 0, 0, 9}
	existing := &common.Path{
		HopCount:    2,
		RandomBlobs: [][]byte{append([]byte(nil), blob...)},
		Expires:     time.Now().Add(time.Hour),
	}
	if !shouldUpdateAnnouncePath(existing, announcePathInput{
		destinationKnown: true,
		announceHops:     2,
		randomBlob:       blob,
		now:              time.Now(),
		announceAffinity: 10,
		currentAffinity:  1,
		affinityKnown:    true,
	}, false) {
		t.Fatal("same emit timebase with higher affinity should update path")
	}
	if shouldUpdateAnnouncePath(existing, announcePathInput{
		destinationKnown: true,
		announceHops:     2,
		randomBlob:       blob,
		now:              time.Now(),
		announceAffinity: 1,
		currentAffinity:  10,
		affinityKnown:    true,
	}, false) {
		t.Fatal("lower affinity should not replace path")
	}
}

func TestRebalancePathHopsDampening(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true, AllowLinkPathRebalance: true})
	iface := &interfaces.UDPInterface{}
	iface.BaseInterface = interfaces.NewBaseInterface("wan", common.IFTypeUDP, true)
	iface.Online = true
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatalf("register: %v", err)
	}
	dest := make([]byte, 16)
	dest[0] = 0xab
	now := time.Now()
	tr.mutex.Lock()
	tr.updatePathUnlocked(dest, dest, iface.GetName(), 2, nil, nil, now)
	tr.mutex.Unlock()

	for i := range rebalanceMaxPerDest {
		if !tr.applyPathHopRebalance(dest, uint8(3+i%2), iface) {
			t.Fatalf("expected rebalance %d to succeed", i)
		}
	}
	if tr.applyPathHopRebalance(dest, 5, iface) {
		t.Fatal("expected dampening to refuse further rebalances in window")
	}
}

func TestRebalancePathHopsRefusesHopIncreaseOntoWeakerGravity(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true, AllowLinkPathRebalance: true})
	strong := &interfaces.UDPInterface{}
	strong.BaseInterface = interfaces.NewBaseInterface("strong", common.IFTypeUDP, true)
	strong.Gravity = 20
	strong.Online = true
	weak := &interfaces.UDPInterface{}
	weak.BaseInterface = interfaces.NewBaseInterface("weak", common.IFTypeUDP, true)
	weak.Gravity = 0
	weak.Online = true
	if err := tr.RegisterInterface("strong", strong); err != nil {
		t.Fatalf("register strong: %v", err)
	}
	if err := tr.RegisterInterface("weak", weak); err != nil {
		t.Fatalf("register weak: %v", err)
	}
	dest := make([]byte, 16)
	dest[0] = 0xcd
	now := time.Now()
	tr.mutex.Lock()
	tr.updatePathUnlocked(dest, dest, strong.GetName(), 1, nil, nil, now)
	tr.mutex.Unlock()

	if tr.applyPathHopRebalance(dest, 4, weak) {
		t.Fatal("hop-increasing rebalance onto much weaker gravity should be refused")
	}
	tr.mutex.RLock()
	hops := tr.paths[pathMapKey(dest)].HopCount
	tr.mutex.RUnlock()
	if hops != 1 {
		t.Fatalf("hop count = %d, want 1", hops)
	}
}

func TestAnnouncesToInternalAllowsBoundaryFeed(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	internal := &interfaces.UDPInterface{}
	internal.BaseInterface = interfaces.NewBaseInterface("internal", common.IFTypeUDP, true)
	internal.Mode = common.IFModeInternal
	internal.Online = true
	boundary := &interfaces.UDPInterface{}
	boundary.BaseInterface = interfaces.NewBaseInterface("boundary", common.IFTypeUDP, true)
	boundary.Mode = common.IFModeBoundary
	boundary.AnnouncesToInternal = true
	boundary.Online = true
	dest := make([]byte, 16)
	dest[0] = 3
	if !tr.shouldForwardAnnounceOn(dest, internal, boundary) {
		t.Fatal("boundary with announces_to_internal should feed internal")
	}
	boundary.AnnouncesToInternal = false
	if tr.shouldForwardAnnounceOn(dest, internal, boundary) {
		t.Fatal("boundary without announces_to_internal should not feed internal")
	}
}

func TestDiscoverySearchModeFilterBoundary(t *testing.T) {
	bi := interfaces.NewBaseInterface("edge", common.IFTypeUDP, true)
	bi.Mode = common.IFModeBoundary
	if !ifaceDiscoversUnknownPaths(&bi) {
		t.Fatal("boundary mode should discover unknown paths")
	}
	filter := discoverySearchModeFilter(&bi)
	if len(filter) != 2 {
		t.Fatalf("boundary filter len = %d, want 2", len(filter))
	}
	bi.RecursivePRs = true
	if discoverySearchModeFilter(&bi) != nil {
		t.Fatal("recursive_prs boundary should not filter modes")
	}
}
