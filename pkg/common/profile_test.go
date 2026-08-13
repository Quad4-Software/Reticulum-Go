// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"runtime"
	"testing"
)

func TestApplyNodeProfileCoreRouterFillsUnset(t *testing.T) {
	c := &ReticulumConfig{
		NodeProfile:   NodeProfileCoreRouter,
		DoSProtection: "auto",
	}
	c.ApplyNodeProfile()
	if c.DoSProtection != "prevent" {
		t.Fatalf("dos=%q want prevent", c.DoSProtection)
	}
	if !c.WatchInterfaces {
		t.Fatal("watch_interfaces should be on")
	}
	if c.BackboneIO != "auto" {
		t.Fatalf("backbone_io=%q want auto", c.BackboneIO)
	}
	wantHandlers := max(runtime.GOMAXPROCS(0)*64, DefaultMaxPacketHandlers)
	if c.MaxPacketHandlers != wantHandlers {
		t.Fatalf("handlers=%d want %d", c.MaxPacketHandlers, wantHandlers)
	}
	if c.MaxInMemoryPaths != CoreRouterMaxInMemoryPaths {
		t.Fatalf("paths=%d want %d", c.MaxInMemoryPaths, CoreRouterMaxInMemoryPaths)
	}
}

func TestApplyNodeProfileDoesNotOverrideExplicit(t *testing.T) {
	c := &ReticulumConfig{
		NodeProfile:          NodeProfileCoreRouter,
		DoSProtection:        "detect",
		DoSProtectionSet:     true,
		WatchInterfaces:      false,
		WatchInterfacesSet:   true,
		MaxPacketHandlers:    7,
		MaxPacketHandlersSet: true,
		MaxInMemoryPaths:     11,
		MaxInMemoryPathsSet:  true,
	}
	c.ApplyNodeProfile()
	if c.DoSProtection != "detect" {
		t.Fatalf("dos overwritten to %q", c.DoSProtection)
	}
	if c.WatchInterfaces {
		t.Fatal("watch_interfaces overwritten")
	}
	if c.MaxPacketHandlers != 7 {
		t.Fatalf("handlers overwritten to %d", c.MaxPacketHandlers)
	}
	if c.MaxInMemoryPaths != 11 {
		t.Fatalf("paths overwritten to %d", c.MaxInMemoryPaths)
	}
}

func TestApplyNodeProfileEmbedded(t *testing.T) {
	c := &ReticulumConfig{NodeProfile: NodeProfileEmbedded}
	c.ApplyNodeProfile()
	if c.MaxPacketHandlers != EmbeddedMaxPacketHandlers {
		t.Fatalf("handlers=%d", c.MaxPacketHandlers)
	}
	if c.MaxInMemoryPaths != EmbeddedMaxInMemoryPaths {
		t.Fatalf("paths=%d", c.MaxInMemoryPaths)
	}
	if c.MaxInMemoryKnownDestinations != EmbeddedMaxInMemoryKnownDestinations {
		t.Fatalf("known=%d", c.MaxInMemoryKnownDestinations)
	}
	if c.MaxPacketHashlist != EmbeddedMaxPacketHashlist {
		t.Fatalf("hashlist=%d", c.MaxPacketHashlist)
	}
}

func TestEffectiveMaxPacketHandlers(t *testing.T) {
	if (*ReticulumConfig)(nil).EffectiveMaxPacketHandlers() != DefaultMaxPacketHandlers {
		t.Fatal("nil config")
	}
	c := &ReticulumConfig{}
	if c.EffectiveMaxPacketHandlers() != DefaultMaxPacketHandlers {
		t.Fatal("zero should default")
	}
	c.MaxPacketHandlers = 9
	if c.EffectiveMaxPacketHandlers() != 9 {
		t.Fatal("explicit")
	}
}
