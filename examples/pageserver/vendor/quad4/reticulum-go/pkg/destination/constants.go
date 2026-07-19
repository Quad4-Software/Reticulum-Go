// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package destination

const (
	In  = 0x01
	Out = 0x02

	Single = 0x00
	SINGLE = Single // FIXME: remove this when reticulum-mf is updated.
	Group  = 0x01
	Plain  = 0x02

	ProveNone = 0x00
	ProveAll  = 0x01
	ProveApp  = 0x02

	AllowNone = 0x00
	AllowAll  = 0x01
	AllowList = 0x02

	RatchetCount    = 512
	RatchetInterval = 1800
)
