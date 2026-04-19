// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package transport

import "time"

const (
	PathfinderM     = 128
	PathRequestTTL  = 300
	AnnounceTimeout = 15

	// SeenAnnounceTTL is how long a deduplication key for an announce hash is retained.
	SeenAnnounceTTL = 1 * time.Hour

	// MaxConcurrentPacketHandlers limits concurrent goroutines spawned by HandlePacket.
	MaxConcurrentPacketHandlers = 512

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

	// PathfinderRW is the random window (seconds) added before
	// retransmitting an announce.
	PathfinderRW = 0.5

	// PathfinderR is the number of retransmit retries for queued
	// announces. Matches Transport.PATHFINDER_R.
	PathfinderR = 1

	// PathfinderG is the retry grace period in seconds added to the
	// retransmit timeout. Matches Transport.PATHFINDER_G.
	PathfinderG = 5

	// PathRequestMI is the minimum interval between automated path
	// requests for the same destination. Matches Transport.PATH_REQUEST_MI.
	PathRequestMI = 20 * time.Second

	// LocalRebroadcastsMax bounds how many local rebroadcasts of a
	// queued announce are allowed before it is considered handed off.
	// Matches Transport.LOCAL_REBROADCASTS_MAX.
	LocalRebroadcastsMax = 2

	// LinkProofTimeoutPerHop is the link-establishment proof timeout
	// added per remaining hop when registering a relayed link entry.
	LinkProofTimeoutPerHop = 6 * time.Second

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
