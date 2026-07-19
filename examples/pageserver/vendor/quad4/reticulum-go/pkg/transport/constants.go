// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import "time"

const (
	PathfinderM     = 128
	PathRequestTTL  = 300
	AnnounceTimeout = 15

	// PathfinderE is path table lifetime in seconds (Python PATHFINDER_E).
	PathfinderE = 60 * 60 * 24 * 7

	// APPathTime is path lifetime for Access Point mode interfaces.
	APPathTime = 24 * time.Hour

	// RoamingPathTime is path lifetime for Roaming mode interfaces.
	RoamingPathTime = 6 * time.Hour

	// HashlistMaxSize caps the in-memory packet hash loop filter
	// (Python Transport.hashlist_maxsize).
	HashlistMaxSize = 1_000_000

	// ReverseTimeout is how long reverse-table proof return paths are kept.
	ReverseTimeout = 8 * 60 * time.Second

	// SeenAnnounceTTL is how long a deduplication key for an announce hash is retained.
	SeenAnnounceTTL = 1 * time.Hour

	// MaxConcurrentPacketHandlers limits concurrent goroutines spawned by HandlePacket.
	MaxConcurrentPacketHandlers = 512

	MaxRegisteredLinks = 256

	// MaxPendingAnnounceForwards caps delayed announce rebroadcast jobs queued
	// for the announce-forward ticker. Avoids per-announce sleep goroutines that
	// explode memory and OS threads under storms.
	MaxPendingAnnounceForwards = 256

	EstablishmentTimeoutPerHop = 6
	KeepaliveTimeoutFactor     = 4
	StaleGrace                 = 2
	Keepalive                  = 360
	StaleTime                  = 720
	// LinkTimeout is idle lifetime for validated link-table rows
	// (Python LINK_TIMEOUT = STALE_TIME * 1.25).
	LinkTimeout = time.Duration(StaleTime*5/4) * time.Second

	AcceptNone = 0
	AcceptAll  = 1
	AcceptApp  = 2

	ResourceStatusPending   = 0x00
	ResourceStatusActive    = 0x01
	ResourceStatusComplete  = 0x02
	ResourceStatusFailed    = 0x03
	ResourceStatusCancelled = 0x04

	Out = 0x02
	In  = 0x01

	Single = 0x00
	Group  = 0x01
	Plain  = 0x02

	StatusNew    = 0
	StatusActive = 1
	StatusClosed = 2
	StatusFailed = 3

	AnnounceRatePercent = 2.0
	AnnounceRateKbps    = 20.0

	MaxHops         = 128
	PropagationRate = 0.02

	// PathfinderRW is the random window (seconds) added before
	// retransmitting an announce.
	PathfinderRW = 0.5

	// PathfinderR is the number of retransmit retries for queued
	// announces.
	PathfinderR = 1

	// PathfinderG is the retry grace period in seconds added to the
	// retransmit timeout.
	PathfinderG = 5

	// PathRequestMI is the minimum interval between automated path
	// requests for the same destination.
	PathRequestMI = 20 * time.Second

	// maxQueuedDiscoveryPRs is the maximum pending discovery path requests.
	maxQueuedDiscoveryPRs = 32

	// discoveryPRTxThrottle is the minimum interval between processing
	// queued discovery path requests.
	discoveryPRTxThrottle = 500 * time.Millisecond

	// LocalRebroadcastsMax bounds how many local rebroadcasts of a
	// queued announce are allowed before it is considered handed off.
	LocalRebroadcastsMax = 2

	// LinkProofTimeoutPerHop is the link-establishment proof timeout
	// added per remaining hop when registering a relayed link entry.
	LinkProofTimeoutPerHop = 6 * time.Second

	PacketTypeAnnounce = 0x01
	PacketTypeLink     = 0x02

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
)

const (
	MaxRetries             = 3
	RetryInterval          = 5 * time.Second
	MaxQueueSize           = 1000
	MinPriorityDelta       = 0.1
	DefaultPropagationRate = 0.02
)

const (
	StateUnknown      = 0x00
	StateUnresponsive = 0x01
	StateResponsive   = 0x02
)
