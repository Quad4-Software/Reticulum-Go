// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package destination

const (
	IN  = 0x01
	OUT = 0x02

	SINGLE = 0x00
	GROUP  = 0x01
	PLAIN  = 0x02

	PROVE_NONE = 0x00
	PROVE_ALL  = 0x01
	PROVE_APP  = 0x02

	ALLOW_NONE = 0x00
	ALLOW_ALL  = 0x01
	ALLOW_LIST = 0x02

	RATCHET_COUNT    = 512
	RATCHET_INTERVAL = 1800
)
