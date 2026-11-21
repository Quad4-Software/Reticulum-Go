// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

// AppName is the Reticulum destination application name for RRC hubs.
const AppName = "rrc"

// HubAspect is the destination aspect for RRC hubs (rrc.hub).
const HubAspect = "hub"

// ProtocolVersion is the RRC on-wire protocol version for this implementation.
const ProtocolVersion uint64 = 1

// Fixed wire field sizes.
const (
	MessageIDLength  = 8
	IdentityLength   = 16
	FixedEnvelopeMin = 43
)

// Envelope field keys (3-RRC).
const (
	KeyVersion   uint64 = 0
	KeyType      uint64 = 1
	KeyMsgID     uint64 = 2
	KeyTimestamp uint64 = 3
	KeySender    uint64 = 4
	KeyRoom      uint64 = 5
	KeyBody      uint64 = 6
	KeyNick      uint64 = 7
	// KeyDestination is an rrcd extension for direct NOTICE (envelope key 8).
	KeyDestination uint64 = 8
)

// Message type assignments (3-RRC).
const (
	TypeHello   uint64 = 1
	TypeWelcome uint64 = 2
	TypeJoin    uint64 = 10
	TypeJoined  uint64 = 11
	TypePart    uint64 = 12
	TypeParted  uint64 = 13
	TypeMsg     uint64 = 20
	TypeNotice  uint64 = 21
	TypeAction  uint64 = 22
	TypePing    uint64 = 30
	TypePong    uint64 = 31
	TypeError   uint64 = 40
	// TypeResourceEnvelope is an rrcd extension (type 50) for resource metadata.
	TypeResourceEnvelope uint64 = 50
)

// HELLO body keys.
const (
	HelloKeyClientName    uint64 = 0
	HelloKeyClientVersion uint64 = 1
	HelloKeyCapabilities  uint64 = 2
	// HelloKeyNickLegacy is a pre-spec HELLO body nick (prefer envelope KeyNick).
	HelloKeyNickLegacy uint64 = 64
)

// WELCOME body keys.
const (
	WelcomeKeyHubName      uint64 = 0
	WelcomeKeyHubVersion   uint64 = 1
	WelcomeKeyCapabilities uint64 = 2
	WelcomeKeyHubLimits    uint64 = 3
)

// HubLimits nested map keys (within WELCOME body key 3), matching rrcd.
const (
	LimitMaxNickBytes           uint64 = 0
	LimitMaxRoomNameBytes       uint64 = 1
	LimitMaxMsgBodyBytes        uint64 = 2
	LimitMaxRoomsPerSession     uint64 = 3
	LimitRateLimitMsgsPerMinute uint64 = 4
)

// Capability map keys in HELLO/WELCOME body key 2 (rrcd advisory flags).
const (
	CapResourceEnvelope uint64 = 0
	CapAction           uint64 = 1
	CapDirectNotice     uint64 = 2
)

// RESOURCE_ENVELOPE body keys.
const (
	ResourceKeyID       uint64 = 0
	ResourceKeyKind     uint64 = 1
	ResourceKeySize     uint64 = 2
	ResourceKeySHA256   uint64 = 3
	ResourceKeyEncoding uint64 = 4
)

// Resource kind strings used by rrcd.
const (
	ResourceKindNotice = "notice"
	ResourceKindMOTD   = "motd"
	ResourceKindBlob   = "blob"
)

// Default hub limits used when HubConfig leaves a field at zero.
const (
	DefaultMaxNickBytes           = 32
	DefaultMaxRoomsPerSession     = 32
	DefaultMaxRoomNameBytes       = 64
	DefaultMaxMsgBodyBytes        = 350
	DefaultRateLimitMsgsPerMinute = 60
	DefaultMaxResourceBytes       = 262144
	// MaxEnvelopeBytes is the inbound CBOR envelope size cap.
	MaxEnvelopeBytes = 64 << 10
)
