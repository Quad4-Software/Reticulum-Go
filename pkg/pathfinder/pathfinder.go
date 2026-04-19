// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package pathfinder

import "time"

// PathFinder stores and looks up paths to destinations by hash.
type PathFinder struct {
	paths map[string]Path
}

// Path holds the next hop, interface name, hop count and last update time for a destination.
type Path struct {
	NextHop     []byte
	Interface   string
	HopCount    byte
	LastUpdated int64
}

// NewPathFinder returns a new PathFinder.
func NewPathFinder() *PathFinder {
	return &PathFinder{
		paths: make(map[string]Path),
	}
}

// AddPath records or updates the path for the given destination hash.
func (p *PathFinder) AddPath(destHash string, nextHop []byte, iface string, hops byte) {
	p.paths[destHash] = Path{
		NextHop:     nextHop,
		Interface:   iface,
		HopCount:    hops,
		LastUpdated: time.Now().Unix(),
	}
}

// GetPath returns the path for the given destination hash, if any.
func (p *PathFinder) GetPath(destHash string) (Path, bool) {
	path, exists := p.paths[destHash]
	return path, exists
}
