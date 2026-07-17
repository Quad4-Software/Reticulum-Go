// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

// Sizes and propagation/destination type constants.
const (
	EncryptedMDU = 383
	PlainMDU     = 464

	PropagationBroadcast = 0
	PropagationTransport = 1

	DestinationSingle = 0
	DestinationGroup  = 1
	DestinationPlain  = 2
	DestinationLink   = 3

	MinAnnounceSize = 170

	TruncatedHashLength = 16
	MinPacketSize       = 3

	PathfinderM = 128

	// LinkRequestECPubSize is the combined X25519 + Ed25519 public key size in link requests.
	LinkRequestECPubSize = 64

	ReceiptTimeoutBaseSec   = 15
	ReceiptTimeoutPerHopSec = 3
)

// Header flag bit masks for packing/unpacking.
const (
	HeaderMaskHeaderType      = 0x40
	HeaderMaskContextFlag     = 0x20
	HeaderMaskTransportType   = 0x10
	HeaderMaskDestinationType = 0x0C
	HeaderMaskPacketType      = 0x03
	HashableFlagsMask         = 0x0F
)

// Packet types, header types, context and flag constants.
const (
	PacketTypeData     = 0x00
	PacketTypeAnnounce = 0x01
	PacketTypeLinkReq  = 0x02
	PacketTypeProof    = 0x03

	HeaderType1 = 0x00
	HeaderType2 = 0x01

	ContextNone         = 0x00
	ContextResource     = 0x01
	ContextResourceAdv  = 0x02
	ContextResourceReq  = 0x03
	ContextResourceHMU  = 0x04
	ContextResourcePRF  = 0x05
	ContextResourceICL  = 0x06
	ContextResourceRCL  = 0x07
	ContextCacheReq     = 0x08
	ContextRequest      = 0x09
	ContextResponse     = 0x0A
	ContextPathResponse = 0x0B
	ContextCommand      = 0x0C
	ContextCmdStatus    = 0x0D
	ContextChannel      = 0x0E
	ContextKeepalive    = 0xFA
	ContextLinkIdentify = 0xFB
	ContextLinkClose    = 0xFC
	ContextLinkProof    = 0xFD
	ContextLRRTT        = 0xFE
	ContextLRProof      = 0xFF

	FlagSet   = 0x01
	FlagUnset = 0x00

	HeaderMaxSize = 64
	MTU           = 500

	MaxInboundPacketSize = 262144

	AddressSize = 32
)
