// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

// RPC wrappers for the destination-type filter surface (RNS 1.6.0).

func (t *Transport) WantsDestinationTypeRPC(fullName, requester string) bool {
	if t == nil || t.destFilters == nil {
		return false
	}
	return t.destFilters.wants(fullName, requester)
}

func (t *Transport) UnwantsDestinationTypeRPC(fullName, requester string) bool {
	if t == nil || t.destFilters == nil {
		return false
	}
	return t.destFilters.unwants(fullName, requester)
}

func (t *Transport) UnwantsAllDestinationTypesRPC(requester string) bool {
	if t == nil || t.destFilters == nil {
		return false
	}
	return t.destFilters.unwantsAll(requester)
}

// LinkDistributionRPC mirrors Python Transport.link_distribution(), which is
// a stub returning {} upstream; keep the same shape for RPC parity.
func (t *Transport) LinkDistributionRPC() map[string]any {
	return map[string]any{}
}
