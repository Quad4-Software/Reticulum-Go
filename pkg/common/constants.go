// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

// Interface type discriminators.
const (
	IFTypeNone InterfaceType = iota
	IFTypeUDP
	IFTypeTCP
	IFTypeUnix
	IFTypeI2P
	IFTypeBluetooth
	IFTypeSerial
	IFTypeAuto
	IFTypeBackbone
	IFTypePipe
	IFTypeQUIC
	IFTypeWebTransport
	IFTypeDNSRendezvous
	IFTypeVSOCK
	IFTypeHTTPS
	IFTypeModem73
	IFTypeSDR
)

// Interface operational modes (wire values).
const (
	IFModeFull        InterfaceMode = 0x01
	IFModePoint       InterfaceMode = 0x02
	IFModeAccessPoint InterfaceMode = 0x03
	IFModeRoaming     InterfaceMode = 0x04
	IFModeBoundary    InterfaceMode = 0x05
	IFModeGateway     InterfaceMode = 0x06
	IFModeInternal    InterfaceMode = 0x07
)

// DiscoverPathsFor lists modes that trigger recursive path discovery for
// unknown destinations.
var DiscoverPathsFor = []InterfaceMode{
	IFModeAccessPoint,
	IFModeGateway,
	IFModeRoaming,
	IFModeInternal,
}

// Transport modes.
const (
	TransportModeDirect TransportMode = iota
	TransportModeRelay
	TransportModeGateway
)

// Path status.
const (
	PathStatusUnknown PathStatus = iota
	PathStatusDirect
	PathStatusRelay
	PathStatusFailed
)

// Resource status codes for top-level resource transfers.
const (
	ResourceStatusPending   = 0x00
	ResourceStatusActive    = 0x01
	ResourceStatusComplete  = 0x02
	ResourceStatusFailed    = 0x03
	ResourceStatusCancelled = 0x04
)

// Link status codes.
const (
	LinkStatusPending = 0x00
	LinkStatusActive  = 0x01
	LinkStatusClosed  = 0x02
	LinkStatusFailed  = 0x03
)

// Direction bit flags used by destinations and interfaces.
const (
	In  = 0x01
	Out = 0x02
)

// Default sizing and rate-limit values.
const (
	DefaultMTU     = 1500
	MaxPacketSize  = 65535
	BitrateMinimum = 5
)

// Timeouts and intervals (seconds unless otherwise noted).
const (
	EstablishTimeout  = 6
	KeepaliveInterval = 360
	StaleTime         = 720
	PathRequestTTL    = 300
	AnnounceTimeout   = 15
)

// TokenCipher overhead in bytes (IV + auth tag area).
const TokenOverhead = 48

// Port range for IP-based interfaces.
const (
	MinPort = 1
	MaxPort = 65535
)

// Default service ports and log level for the local instance.
const (
	DefaultSharedInstancePort  = 37428
	DefaultInstanceControlPort = 37429
	DefaultLogLevel            = 4
)

// Defaults for the localhost control API (pkg/controlapi).
const (
	DefaultControlAPIHost = "127.0.0.1"
	DefaultControlAPIPort = 37430
)

// Destination type discriminators encoded in packet headers.
const (
	DestinationSingle = 0x00
	DestinationGroup  = 0x01
	DestinationPlain  = 0x02
)
