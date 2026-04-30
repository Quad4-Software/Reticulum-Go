// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package transport

// Wire-format constants for Reticulum packet headers.
//
// A Reticulum header byte is laid out as:
//
//	bit 7      : IFAC flag           (HeaderIFACMask)
//	bit 6      : header type         (HeaderTypeMask)        0=Type1, 1=Type2
//	bit 5      : context flag        (HeaderContextFlagMask)
//	bit 4      : propagation type    (HeaderPropTypeMask)
//	bits 3..2  : destination type    (HeaderDestTypeMask)
//	bits 1..0  : packet type         (HeaderPacketTypeMask)
const (
	HeaderIFACMask        byte = 0x80
	HeaderTypeMask        byte = 0x40
	HeaderContextFlagMask byte = 0x20
	HeaderPropTypeMask    byte = 0x10
	HeaderDestTypeMask    byte = 0x0C
	HeaderPacketTypeMask  byte = 0x03

	HeaderIFACShift        = 7
	HeaderTypeShift        = 6
	HeaderContextFlagShift = 5
	HeaderPropTypeShift    = 4
	HeaderDestTypeShift    = 2
)

// Packet field sizes and offsets shared between transport, announce and
// path-request handling. Changing any of these requires a coordinated
// wire-format bump.
const (
	HeaderSize     = 2  // first two bytes: flags + hop count
	AddrHashSize   = 16 // single-address field (HeaderType1)
	DoubleAddrSize = 32 // two address fields  (HeaderType2)
	ContextByteLen = 1
	HopCountOffset = 1

	// MinHeaderedPacketSize is the smallest valid packet that still has
	// a header byte and a hop counter.
	MinHeaderedPacketSize = HeaderSize

	// MinTransportPacketSize is the smallest size for a HeaderType1
	// data packet that includes a context byte.
	MinTransportPacketSize = HeaderSize + AddrHashSize + ContextByteLen // 19

	// MinPathResponseSize is the smallest valid path-response payload
	// (full destination hash + 1 hop byte).
	MinPathResponseSize = 32 + 1

	// MinAnnouncePacketSize is the smallest accepted announce packet
	// across both header types (defensive lower bound used by the
	// transport-layer dispatcher; the announce parser performs a
	// stricter, format-specific check).
	MinAnnouncePacketSize = 53

	// SuspiciousLinkPacketSize is logged at error level because a
	// 67-byte packet historically indicates a malformed/duplicate link
	// frame. Tracked separately so the heuristic can be tuned.
	SuspiciousLinkPacketSize = 67
)

// Wire bytes used when assembling msgpack payloads inside announces.
//
// We hand-roll a small, fixed msgpack envelope (a 2-element array of bin8
// values) instead of pulling in the encoder for hot-path code.
const (
	MsgpackArray2     byte = 0x92
	MsgpackBin8       byte = 0xc4
	MsgpackBin8MaxLen      = 255
)

// Single-byte boolean wire encoding used in path-request frames.
const (
	wireFlagFalse byte = 0x00
	wireFlagTrue  byte = 0x01
)

// Tunables for transport bookkeeping.
const (
	// DiscoveryPRTagsCap bounds the in-memory dedupe set for
	// discovery path-request tags before it is rotated.
	DiscoveryPRTagsCap = 32000

	// PathRequestTTLMax is the maximum inclusive TTL that fits in the
	// single-byte TTL field of a path request.
	PathRequestTTLMax = 255

	// AnnounceNameHashSize is the truncated SHA-256 length of the
	// destination name embedded in an announce.
	AnnounceNameHashSize = 10

	// AnnounceRandomHashSize is the size of the per-announce random
	// hash. It is composed of AnnounceRandomBytesLen random bytes
	// followed by AnnounceTimestampBytesLen truncated timestamp bytes.
	AnnounceRandomHashSize    = 10
	AnnounceRandomBytesLen    = 5
	AnnounceTimestampBytesLen = 5
)
