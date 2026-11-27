// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
)

// applyFilterConfig applies the parsed [filter] section (RNS 1.6.0):
// ignored_types to block_destination_type, allowed_types to
// allow_destination_type, required_only to required_destinations_only.
func (t *Transport) applyFilterConfig(f common.FilterConfig) {
	if t == nil || t.destFilters == nil {
		return
	}
	for _, name := range f.IgnoredTypes {
		t.destFilters.block(name)
		debug.Log(debug.DebugInfo, "Blocked destination type", "type", name)
	}
	for _, name := range f.AllowedTypes {
		t.destFilters.allow(name)
	}
	if f.RequiredOnlySet {
		t.filterRequiredOnly = f.RequiredOnly
	}
	if f.HopLimiterSet {
		t.filterHopLimiter = f.HopLimiter
	}
}
