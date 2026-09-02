// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"runtime"
	"strings"
)

const (
	NodeProfileDefault    = "default"
	NodeProfileCoreRouter = "core_router"
	NodeProfileEmbedded   = "embedded"

	SandboxProfileFull   = "full"
	SandboxProfileRouter = "router"
)

// ApplyNodeProfile fills unset knobs from NodeProfile. Explicit config keys
// always win.
func (c *ReticulumConfig) ApplyNodeProfile() {
	if c == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(c.NodeProfile)) {
	case NodeProfileCoreRouter:
		c.applyCoreRouterProfile()
	case NodeProfileEmbedded:
		c.applyEmbeddedProfile()
	}
}

func (c *ReticulumConfig) applyCoreRouterProfile() {
	if !c.BackboneIOSet && strings.TrimSpace(c.BackboneIO) == "" {
		c.BackboneIO = "auto"
	}
	if !c.WatchInterfacesSet {
		c.WatchInterfaces = true
	}
	if !c.MaxPacketHandlersSet {
		n := max(runtime.GOMAXPROCS(0)*64, DefaultMaxPacketHandlers)
		c.MaxPacketHandlers = n
	}
	if !c.MaxInMemoryPathsSet && c.MaxInMemoryPaths == 0 {
		c.MaxInMemoryPaths = CoreRouterMaxInMemoryPaths
	}
	if !c.MaxPacketHashlistSet && c.MaxPacketHashlist == 0 {
		c.MaxPacketHashlist = DefaultMaxPacketHashlist
	}
}

func (c *ReticulumConfig) applyEmbeddedProfile() {
	if !c.MaxPacketHandlersSet {
		c.MaxPacketHandlers = EmbeddedMaxPacketHandlers
	}
	if !c.MaxInMemoryPathsSet && c.MaxInMemoryPaths == 0 {
		c.MaxInMemoryPaths = EmbeddedMaxInMemoryPaths
	}
	if !c.MaxInMemoryKnownDestinationsSet && c.MaxInMemoryKnownDestinations == 0 {
		c.MaxInMemoryKnownDestinations = EmbeddedMaxInMemoryKnownDestinations
	}
	if !c.MaxPacketHashlistSet && c.MaxPacketHashlist == 0 {
		c.MaxPacketHashlist = EmbeddedMaxPacketHashlist
	}
}
