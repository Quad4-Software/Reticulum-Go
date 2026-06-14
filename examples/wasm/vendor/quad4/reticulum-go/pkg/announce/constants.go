// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package announce

// Packet types, announce types, header/dest/propagation and interface auth constants.
const (
	PacketTypeData     = 0x00
	PacketTypeAnnounce = 0x01
	PacketTypeLink     = 0x02
	PacketTypeProof    = 0x03

	AnnounceNone     = 0x00
	AnnouncePath     = 0x01
	AnnounceIdentity = 0x02

	HeaderType1 = 0x00
	HeaderType2 = 0x01

	PropTypeBroadcast = 0x00
	PropTypeTransport = 0x01

	DestTypeSingle = 0x00
	DestTypeGroup  = 0x01
	DestTypePlain  = 0x02
	DestTypeLink   = 0x03

	IFACNone = 0x00
	IFACAuth = 0x80

	MaxHops         = 128
	PropagationRate = 0.02
	RetryInterval   = 300
	MaxRetries      = 3
)

// Wire-format sizes for announce packets. These mirror the on-disk layout
// described in the Reticulum protocol specification; changing any of them is a
// protocol break.
const (
	HeaderSize     = 2  // flags + hop count
	AddrHashSize   = 16 // single destination/transport hash
	ContextByteLen = 1

	// HeaderType1Offset is the offset at which payload begins for a
	// HeaderType1 packet (header + dest hash + context byte).
	HeaderType1Offset = HeaderSize + AddrHashSize + ContextByteLen // 19

	// HeaderType2Offset is the offset at which payload begins for a
	// HeaderType2 packet (header + dest + transport id + context).
	HeaderType2Offset = HeaderSize + 2*AddrHashSize + ContextByteLen // 35

	// MinHeaderType1Size and MinHeaderType2Size are the smallest
	// possible packet lengths that still contain the full header for
	// each variant.
	MinHeaderType1Size = HeaderType1Offset
	MinHeaderType2Size = HeaderType2Offset

	PubKeyHalfSize = 32 // each of the encryption / signing pub keys
	PubKeySize     = 2 * PubKeyHalfSize
	NameHashSize   = 10
	RandomHashSize = 10
	RatchetSize    = 32
	SignatureSize  = 64

	// Field offsets within an announce data block (after header + addresses).
	AnnounceEncKeyOffset    = 0
	AnnounceSignKeyOffset   = AnnounceEncKeyOffset + PubKeyHalfSize   // 32
	AnnounceNameHashOffset  = AnnounceSignKeyOffset + PubKeyHalfSize  // 64
	AnnounceRandomOffset    = AnnounceNameHashOffset + NameHashSize   // 74
	AnnounceRatchetOffset   = AnnounceRandomOffset + RandomHashSize   // 84
	AnnounceSignatureOffset = AnnounceRatchetOffset + RatchetSize     // 116
	AnnounceAppDataOffset   = AnnounceSignatureOffset + SignatureSize // 180

	// MinAnnounceDataSize is the minimum length of the data portion
	// (everything after the header + addresses + context byte): all
	// fixed fields up to and including the signature.
	MinAnnounceDataSize = AnnounceAppDataOffset // 180

	// MinAnnouncePacketSize is the smallest valid announce packet
	// (HeaderType1 framing + minimum data block + 3 bytes app data).
	MinAnnouncePacketSize = MinHeaderType1Size + MinAnnounceDataSize - ContextByteLen + 3 // 170

	// HeaderTypeMask isolates the header-type bit (bit 6) of a header byte.
	HeaderTypeMask  byte = 0b01000000
	HeaderTypeShift      = 6

	// HeaderPacketTypeMask isolates the packet-type bits (lowest 2 bits).
	HeaderPacketTypeMask byte = 0x03
)
