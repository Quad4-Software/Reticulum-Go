// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"time"

	"quad4/reticulum-go/pkg/common"
)

// pathLifetimeFor returns the path-table lifetime for a receiving interface mode
// (Python AP_PATH_TIME / ROAMING_PATH_TIME / PATHFINDER_E).
func pathLifetimeFor(iface common.NetworkInterface) time.Duration {
	if iface == nil {
		return time.Duration(PathfinderE) * time.Second
	}
	switch iface.GetMode() {
	case common.IFModeAccessPoint:
		return APPathTime
	case common.IFModeRoaming:
		return RoamingPathTime
	default:
		return time.Duration(PathfinderE) * time.Second
	}
}
