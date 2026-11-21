// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

const (
	PathOffer         = "/offer"
	PathMessageGet    = "/get"
	PathStatsGet      = "/pn/get/stats"
	PathSyncRequest   = "/pn/peer/sync"
	PathUnpeerRequest = "/pn/peer/unpeer"
)

const (
	PeerErrorNoIdentity   byte = 0xf0
	PeerErrorNoAccess     byte = 0xf1
	PeerErrorInvalidKey   byte = 0xf3
	PeerErrorInvalidData  byte = 0xf4
	PeerErrorInvalidStamp byte = 0xf5
	PeerErrorThrottled    byte = 0xf6
	PeerErrorNotFound     byte = 0xfd
	PeerErrorTimeout      byte = 0xfe
)

const (
	OfferUnknown      byte = 0x00
	OfferAccepted     byte = 0x01
	OfferTransferring byte = 0x02
	OfferValidating   byte = 0x03
)

const (
	PeerStateIdle                 byte = 0x00
	PeerStateLinkEstablishing     byte = 0x01
	PeerStateLinkReady            byte = 0x02
	PeerStateRequestSent          byte = 0x03
	PeerStateResponseReceived     byte = 0x04
	PeerStateResourceTransferring byte = 0x05
)

const (
	PeerStrategyLazy       byte = 0x01
	PeerStrategyPersistent byte = 0x02
)
