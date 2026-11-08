// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !js && !wasip1 && !tinygo

package sqlitestore

import "time"

func float64Now() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}
