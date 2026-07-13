// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
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
