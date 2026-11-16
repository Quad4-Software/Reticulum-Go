// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"sync/atomic"
	"time"
)

type TransportMode byte
type PathStatus byte

type Path struct {
	Interface   NetworkInterface
	NextHop     []byte
	Hops        uint8
	LastUpdated time.Time
	HopCount    uint8
	RandomBlobs [][]byte
	Expires     time.Time

	// usedUnixNano is the last time this path was used for outbound
	// traffic, as Unix nanoseconds (RNS 1.6.0 IDX_PT_USED). Zero means the
	// path was never used: RNS 1.6.0 path table sharding culls never-used
	// paths after a short linger and keeps a bounded pool of them, so the
	// marking happens on the outbound hot path without the transport write
	// lock. Always access through MarkUsed/UsedAt/EverUsed.
	usedUnixNano atomic.Int64
}

// MarkUsed records that the path carried outbound traffic at now.
func (p *Path) MarkUsed(now time.Time) {
	if p != nil {
		p.usedUnixNano.Store(now.UnixNano())
	}
}

// UsedAt returns the last outbound-use time, or the zero time when the
// path was never used.
func (p *Path) UsedAt() time.Time {
	if p == nil {
		return time.Time{}
	}
	if ns := p.usedUnixNano.Load(); ns != 0 {
		return time.Unix(0, ns)
	}
	return time.Time{}
}

// EverUsed reports whether the path has ever carried outbound traffic.
func (p *Path) EverUsed() bool {
	return p != nil && p.usedUnixNano.Load() != 0
}

// Clone returns a deep-enough copy: the usage timestamp carries over so
// sharding state survives copies made to tweak single fields.
func (p *Path) Clone() *Path {
	if p == nil {
		return nil
	}
	cp := &Path{
		Interface:   p.Interface,
		NextHop:     append([]byte(nil), p.NextHop...),
		Hops:        p.Hops,
		LastUpdated: p.LastUpdated,
		HopCount:    p.HopCount,
		Expires:     p.Expires,
	}
	cp.usedUnixNano.Store(p.usedUnixNano.Load())
	for _, blob := range p.RandomBlobs {
		cp.RandomBlobs = append(cp.RandomBlobs, append([]byte(nil), blob...))
	}
	return cp
}

// Common callbacks
type ProofRequestedCallback func([]byte, []byte)
type LinkEstablishedCallback func(any)
type PacketCallback func([]byte, NetworkInterface)

// RequestHandler manages path requests and responses
type RequestHandler struct {
	Path              string
	ResponseGenerator func(path string, data []byte, requestID []byte, linkID []byte, remoteIdentity any, requestedAt int64) any
	AllowMode         byte
	AllowedList       [][]byte
}

// Interface types
type InterfaceMode byte
type InterfaceType byte

// RatchetIDReceiver holds ratchet ID information
type RatchetIDReceiver struct {
	LatestRatchetID []byte
}

// NetworkStats holds interface statistics
type NetworkStats struct {
	BytesSent       uint64
	BytesReceived   uint64
	PacketsSent     uint64
	PacketsReceived uint64
	LastUpdated     time.Time
}

// LinkStatus represents the current state of a link
type LinkStatus struct {
	Established bool
	LastSeen    time.Time
	RTT         time.Duration
	Quality     float64
	Hops        uint8
}

// PathRequest represents a path discovery request
type PathRequest struct {
	DestinationHash []byte
	Tag             []byte
	TTL             int
	Recursive       bool
}

// PathResponse represents a path discovery response
type PathResponse struct {
	DestinationHash []byte
	NextHop         []byte
	Hops            uint8
	Tag             []byte
}
