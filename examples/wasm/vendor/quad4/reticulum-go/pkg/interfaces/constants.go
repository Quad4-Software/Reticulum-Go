// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package interfaces

import "time"

const (
	BitrateMinimum = 1200
	ModeFull       = 0x01

	ModeGateway     = 0x02
	ModeAccessPoint = 0x03
	ModeRoaming     = 0x04
	ModeBoundary    = 0x05

	TypeUDP = 0x01
	TypeTCP = 0x02

	PropagationRate = 0.02
)

const (
	HDLCFlag    = 0x7E
	HDLCEsc     = 0x7D
	HDLCEscMask = 0x20

	KISSFend  = 0xC0
	KISSFesc  = 0xDB
	KISSTFend = 0xDC
	KISSTFesc = 0xDD

	DefaultMTU      = 1064
	BitrateGuessVal = 10 * 1000 * 1000
	ReconnectWait   = 5
	InitialTimeout  = 5
	InitialBackoff  = time.Second
	MaxBackoff      = time.Minute * 5

	TCPUserTimeoutSec   = 24
	TCPProbeAfterSec    = 5
	TCPProbeIntervalSec = 2
	TCPProbesCount      = 12
	TCPConnectTimeout   = 10 * time.Second
	TCPMilliseconds     = 1000

	I2PUserTimeoutSec   = 45
	I2PProbeAfterSec    = 10
	I2PProbeIntervalSec = 9
	I2PProbesCount      = 5

	SOKeepaliveEnable = 1
)

const (
	HWMTU                    = 1196
	DefaultDiscoveryPort     = 29716
	DefaultDataPort          = 42671
	DefaultGroupID           = "reticulum"
	BitrateGuess             = 10 * 1000 * 1000
	PeeringTimeout           = 22 * time.Second
	AndroidTimeoutMultiplier = 3
	AnnounceInterval         = 1600 * time.Millisecond
	PeerJobInterval          = 4 * time.Second
	McastEchoTimeout         = 6500 * time.Millisecond

	ScopeLink         = "2"
	ScopeAdmin        = "4"
	ScopeSite         = "5"
	ScopeOrganisation = "8"
	ScopeGlobal       = "e"

	McastAddrTypePermanent = "0"
	McastAddrTypeTemporary = "1"

	MultiIFDequeLen = 48
	MultiIFDequeTTL = 750 * time.Millisecond
)

const (
	WSMTU = 1064

	// MaxWSControlPayload caps ping/pong/close control frame payloads (defense in depth).
	MaxWSControlPayload = 4096
	WSBitrate           = 10000000
	WSReconnectDelay    = 2 * time.Second
)

const (
	WSBufferSize         = 4096
	WSHTTPSPort          = 443
	WSHTTPPort           = 80
	WSVersion            = "13"
	WSConnectTimeout     = 10 * time.Second
	WSKeySize            = 16
	WSMaskKeySize        = 4
	WSHeaderSize         = 2
	WSPayloadLen16Bit    = 126
	WSPayloadLen64Bit    = 127
	WSMaxPayload16Bit    = 65536
	WSFrameHeaderFin     = 0x80
	WSFrameHeaderOpcode  = 0x0F
	WSFrameHeaderMasked  = 0x80
	WSFrameHeaderLen     = 0x7F
	WSOpcodeContinuation = 0x00
	WSOpcodeText         = 0x01
	WSOpcodeBinary       = 0x02
	WSOpcodeClose        = 0x08
	WSOpcodePing         = 0x09
	WSOpcodePong         = 0x0A
)
