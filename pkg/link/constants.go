// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package link

const (
	Curve = "Curve25519"

	ECPubSize    = 64
	KeySize      = 32
	LinkMTUSize  = 3
	MTUByteMask  = 0xFFFFFF
	ModeByteMask = 0xE0

	EstablishmentTimeoutPerHop = 20
	KeepaliveTimeoutFactor     = 4
	StaleGrace                 = 2
	Keepalive                  = 360
	StaleTime                  = 720
	TrafficTimeoutFactor       = 6

	AcceptNone = 0x00
	AcceptAll  = 0x01
	AcceptApp  = 0x02

	StatusPending   = 0x00
	StatusHandshake = 0x01
	StatusActive    = 0x02
	StatusStale     = 0x03
	StatusClosed    = 0x04
	StatusFailed    = 0x05

	ProveNone = 0x00
	ProveAll  = 0x01
	ProveApp  = 0x02

	ModeAES128CBC = 0x00
	ModeAES256CBC = 0x01
	ModeDefault   = ModeAES256CBC

	WatchdogMinSleep = 0.025
	WatchdogInterval = 0.1

	DestTypeLink = 0x03

	MinRequestDataLen  = 3
	MinResponseDataLen = 2

	// Sentinel hop count meaning "unreachable" returned from
	// Transport.HopsTo. Defined here so call-sites can compare against
	// it without inlining a magic literal.
	HopCountUnreachable uint8 = 0xff

	// Keepalive request/response wire bytes. The non-initiator side of
	// a link replies to a 0xFF probe with 0xFE.
	KeepaliveRequestByte  byte = 0xFF
	KeepaliveResponseByte byte = 0xFE

	// LinkResourceMappedFlag prefixes resource hash payloads that
	// carry a hashmap pointer instead of a raw resource hash.
	LinkResourceMappedFlag byte = 0xFF

	// Msgpack type bytes for the float values used to encode RTT
	// values exchanged on links (float32 / float64 markers).
	MsgpackFloat32Code byte = 0xca
	MsgpackFloat64Code byte = 0xcb

	// Keepalive auto-tuning parameters. Calculated keepalive period is
	// scaled so an RTT of KeepaliveMaxRTT seconds maps to the full
	// Keepalive interval, with a floor of KeepaliveMinSec seconds to
	// avoid hammering low-RTT peers.
	KeepaliveMaxRTT float64 = 1.75
	KeepaliveMinSec float64 = 5.0
)
