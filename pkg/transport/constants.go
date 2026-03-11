// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package transport

import "time"

const (
	PathfinderM     = 128
	PathRequestTTL  = 300
	AnnounceTimeout = 15

	EstablishmentTimeoutPerHop = 6
	KeepaliveTimeoutFactor     = 4
	StaleGrace                 = 2
	Keepalive                  = 360
	StaleTime                  = 720

	AcceptNone = 0
	AcceptAll  = 1
	AcceptApp  = 2

	ResourceStatusPending   = 0x00
	ResourceStatusActive    = 0x01
	ResourceStatusComplete  = 0x02
	ResourceStatusFailed    = 0x03
	ResourceStatusCancelled = 0x04

	OUT = 0x02
	IN  = 0x01

	SINGLE = 0x00
	GROUP  = 0x01
	PLAIN  = 0x02

	STATUS_NEW    = 0
	STATUS_ACTIVE = 1
	STATUS_CLOSED = 2
	STATUS_FAILED = 3

	AnnounceRatePercent = 2.0
	PATHFINDER_M        = 8
	AnnounceRateKbps    = 20.0

	MAX_HOPS         = 128
	PROPAGATION_RATE = 0.02

	PACKET_TYPE_ANNOUNCE = 0x01
	PACKET_TYPE_LINK     = 0x02

	ANNOUNCE_NONE     = 0x00
	ANNOUNCE_PATH     = 0x01
	ANNOUNCE_IDENTITY = 0x02

	HEADER_TYPE_1 = 0x00
	HEADER_TYPE_2 = 0x01

	PROP_TYPE_BROADCAST = 0x00
	PROP_TYPE_TRANSPORT = 0x01

	DEST_TYPE_SINGLE = 0x00
	DEST_TYPE_GROUP  = 0x01
	DEST_TYPE_PLAIN  = 0x02
	DEST_TYPE_LINK   = 0x03
)

const (
	MaxRetries             = 3
	RetryInterval          = 5 * time.Second
	MaxQueueSize           = 1000
	MinPriorityDelta       = 0.1
	DefaultPropagationRate = 0.02
)

const (
	STATE_UNKNOWN      = 0x00
	STATE_UNRESPONSIVE = 0x01
	STATE_RESPONSIVE   = 0x02
)
