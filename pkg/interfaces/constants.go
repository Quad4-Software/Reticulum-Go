// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package interfaces

import "time"

const (
	BITRATE_MINIMUM = 1200
	MODE_FULL       = 0x01

	MODE_GATEWAY      = 0x02
	MODE_ACCESS_POINT = 0x03
	MODE_ROAMING      = 0x04
	MODE_BOUNDARY     = 0x05

	TYPE_UDP = 0x01
	TYPE_TCP = 0x02

	PROPAGATION_RATE = 0.02
)

const (
	HDLC_FLAG     = 0x7E
	HDLC_ESC      = 0x7D
	HDLC_ESC_MASK = 0x20

	KISS_FEND  = 0xC0
	KISS_FESC  = 0xDB
	KISS_TFEND = 0xDC
	KISS_TFESC = 0xDD

	DEFAULT_MTU       = 1064
	BITRATE_GUESS_VAL = 10 * 1000 * 1000
	RECONNECT_WAIT    = 5
	INITIAL_TIMEOUT   = 5
	INITIAL_BACKOFF   = time.Second
	MAX_BACKOFF       = time.Minute * 5

	TCP_USER_TIMEOUT_SEC   = 24
	TCP_PROBE_AFTER_SEC    = 5
	TCP_PROBE_INTERVAL_SEC = 2
	TCP_PROBES_COUNT       = 12
	TCP_CONNECT_TIMEOUT    = 10 * time.Second
	TCP_MILLISECONDS       = 1000

	I2P_USER_TIMEOUT_SEC   = 45
	I2P_PROBE_AFTER_SEC    = 10
	I2P_PROBE_INTERVAL_SEC = 9
	I2P_PROBES_COUNT       = 5

	SO_KEEPALIVE_ENABLE = 1
)

const (
	HW_MTU                 = 1196
	DEFAULT_DISCOVERY_PORT = 29716
	DEFAULT_DATA_PORT      = 42671
	DEFAULT_GROUP_ID       = "reticulum"
	BITRATE_GUESS          = 10 * 1000 * 1000
	PEERING_TIMEOUT        = 22 * time.Second
	ANNOUNCE_INTERVAL      = 1600 * time.Millisecond
	PEER_JOB_INTERVAL      = 4 * time.Second
	MCAST_ECHO_TIMEOUT     = 6500 * time.Millisecond

	SCOPE_LINK         = "2"
	SCOPE_ADMIN        = "4"
	SCOPE_SITE         = "5"
	SCOPE_ORGANISATION = "8"
	SCOPE_GLOBAL       = "e"

	MCAST_ADDR_TYPE_PERMANENT = "0"
	MCAST_ADDR_TYPE_TEMPORARY = "1"

	MULTI_IF_DEQUE_LEN = 48
	MULTI_IF_DEQUE_TTL = 750 * time.Millisecond
)

const (
	WS_MTU = 1064

	// MaxWSControlPayload caps ping/pong/close control frame payloads (defense in depth).
	MaxWSControlPayload = 4096
	WS_BITRATE          = 10000000
	WS_RECONNECT_DELAY  = 2 * time.Second
)

const (
	WS_BUFFER_SIZE         = 4096
	WS_HTTPS_PORT          = 443
	WS_HTTP_PORT           = 80
	WS_VERSION             = "13"
	WS_CONNECT_TIMEOUT     = 10 * time.Second
	WS_KEY_SIZE            = 16
	WS_MASK_KEY_SIZE       = 4
	WS_HEADER_SIZE         = 2
	WS_PAYLOAD_LEN_16BIT   = 126
	WS_PAYLOAD_LEN_64BIT   = 127
	WS_MAX_PAYLOAD_16BIT   = 65536
	WS_FRAME_HEADER_FIN    = 0x80
	WS_FRAME_HEADER_OPCODE = 0x0F
	WS_FRAME_HEADER_MASKED = 0x80
	WS_FRAME_HEADER_LEN    = 0x7F
	WS_OPCODE_CONTINUATION = 0x00
	WS_OPCODE_TEXT         = 0x01
	WS_OPCODE_BINARY       = 0x02
	WS_OPCODE_CLOSE        = 0x08
	WS_OPCODE_PING         = 0x09
	WS_OPCODE_PONG         = 0x0A
)
