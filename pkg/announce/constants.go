// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package announce

// Packet types, announce types, header/dest/propagation and interface auth constants.
const (
	PACKET_TYPE_DATA     = 0x00
	PACKET_TYPE_ANNOUNCE = 0x01
	PACKET_TYPE_LINK     = 0x02
	PACKET_TYPE_PROOF    = 0x03

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

	IFAC_NONE = 0x00
	IFAC_AUTH = 0x80

	MAX_HOPS         = 128
	PROPAGATION_RATE = 0.02
	RETRY_INTERVAL   = 300
	MAX_RETRIES      = 3
)
