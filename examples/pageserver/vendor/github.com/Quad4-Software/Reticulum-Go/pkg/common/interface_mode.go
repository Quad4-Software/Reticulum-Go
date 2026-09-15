// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import "slices"

import "strings"

// ParseInterfaceMode maps config mode strings to wire InterfaceMode values.
func ParseInterfaceMode(s string) InterfaceMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "full":
		return IFModeFull
	case "pointtopoint", "point_to_point", "ptp":
		return IFModePoint
	case "access_point", "accesspoint", "ap":
		return IFModeAccessPoint
	case "roaming":
		return IFModeRoaming
	case "boundary":
		return IFModeBoundary
	case "gateway", "gw":
		return IFModeGateway
	case "internal":
		return IFModeInternal
	default:
		return IFModeFull
	}
}

// ModeDiscoversPaths reports whether mode is in DiscoverPathsFor.
func ModeDiscoversPaths(mode InterfaceMode) bool {
	return slices.Contains(DiscoverPathsFor, mode)
}
