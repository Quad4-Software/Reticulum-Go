// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package common

const (
	// Interface Types
	IF_TYPE_NONE InterfaceType = iota
	IF_TYPE_UDP
	IF_TYPE_TCP
	IF_TYPE_UNIX
	IF_TYPE_I2P
	IF_TYPE_BLUETOOTH
	IF_TYPE_SERIAL
	IF_TYPE_AUTO

	// Interface Modes
	IF_MODE_FULL InterfaceMode = iota
	IF_MODE_POINT
	IF_MODE_GATEWAY
	IF_MODE_ACCESS_POINT
	IF_MODE_ROAMING
	IF_MODE_BOUNDARY

	// Transport Modes
	TRANSPORT_MODE_DIRECT TransportMode = iota
	TRANSPORT_MODE_RELAY
	TRANSPORT_MODE_GATEWAY

	// Path Status
	PATH_STATUS_UNKNOWN PathStatus = iota
	PATH_STATUS_DIRECT
	PATH_STATUS_RELAY
	PATH_STATUS_FAILED

	// Resource Status
	RESOURCE_STATUS_PENDING   = 0x00
	RESOURCE_STATUS_ACTIVE    = 0x01
	RESOURCE_STATUS_COMPLETE  = 0x02
	RESOURCE_STATUS_FAILED    = 0x03
	RESOURCE_STATUS_CANCELLED = 0x04

	// Link Status
	LINK_STATUS_PENDING = 0x00
	LINK_STATUS_ACTIVE  = 0x01
	LINK_STATUS_CLOSED  = 0x02
	LINK_STATUS_FAILED  = 0x03

	// Direction Constants
	IN  = 0x01
	OUT = 0x02

	// Common Constants
	DEFAULT_MTU     = 1500
	MAX_PACKET_SIZE = 65535
	BITRATE_MINIMUM = 5

	// Timeouts and Intervals
	ESTABLISH_TIMEOUT  = 6
	KEEPALIVE_INTERVAL = 360
	STALE_TIME         = 720
	PATH_REQUEST_TTL   = 300
	ANNOUNCE_TIMEOUT   = 15

	// Common Numeric Constants
	ZERO    = 0
	ONE     = 1
	TWO     = 2
	THREE   = 3
	FOUR    = 4
	FIVE    = 5
	SIX     = 6
	SEVEN   = 7
	EIGHT   = 8
	FIFTEEN = 15

	// Common Size Constants
	SIZE_16        = 16
	SIZE_32        = 32
	SIZE_48        = 48
	SIZE_64        = 64
	SIXTY_SEVEN    = 67
	TOKEN_OVERHEAD = 48

	// Common Hex Constants
	HEX_0x00 = 0x00
	HEX_0x01 = 0x01
	HEX_0x02 = 0x02
	HEX_0x03 = 0x03
	HEX_0x04 = 0x04
	HEX_0x92 = 0x92
	HEX_0x93 = 0x93
	HEX_0xC2 = 0xC2
	HEX_0xC3 = 0xC3
	HEX_0xC4 = 0xC4
	HEX_0xD1 = 0xD1
	HEX_0xD2 = 0xD2
	HEX_0xFE = 0xFE
	HEX_0xFF = 0xFF

	// Common Numeric Constants
	NUM_11   = 11
	NUM_100  = 100
	NUM_500  = 500
	NUM_1024 = 1024
	NUM_1064 = 1064
	NUM_4242 = 4242
	NUM_0700 = 0700

	// Common Float Constants
	FLOAT_ZERO  = 0.0
	FLOAT_0_001 = 0.001
	FLOAT_0_025 = 0.025
	FLOAT_0_1   = 0.1
	FLOAT_1_0   = 1.0
	FLOAT_1_75  = 1.75
	FLOAT_5_0   = 5.0
	FLOAT_1E9   = 1e9

	// Common String Constants
	STR_LINK_ID           = "link_id"
	STR_BYTES             = "bytes"
	STR_FMT_HEX           = "0x%02x"
	STR_FMT_HEX_LOW       = "%x"
	STR_FMT_DEC           = "%d"
	STR_TEST              = "test"
	STR_LINK              = "link"
	STR_ERROR             = "error"
	STR_HASH              = "hash"
	STR_NAME              = "name"
	STR_TYPE              = "type"
	STR_STORAGE           = "storage"
	STR_PATH              = "path"
	STR_COUNT             = "count"
	STR_HOME              = "HOME"
	STR_PUBLIC_KEY        = "public_key"
	STR_TCP_CLIENT        = "TCPClientInterface"
	STR_UDP               = "udp"
	STR_UDP6              = "udp6"
	STR_TCP               = "tcp"
	STR_ETH0              = "eth0"
	STR_INTERFACE         = "interface"
	STR_PEER              = "peer"
	STR_ADDR              = "addr"
	STR_LINK_NOT_ACTIVE   = "link not active"
	STR_INTERFACE_OFFLINE = "interface offline or detached"
)
