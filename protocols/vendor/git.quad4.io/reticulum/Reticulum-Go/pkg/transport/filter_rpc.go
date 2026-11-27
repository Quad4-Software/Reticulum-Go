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

// WantsDiscoveryDestination registers the rnstransport.discovery.interface
// destination type as required (Python 1.6.1: Discovery start calls
// wants_destination_type so required_only instances still accept discovery
// announces).
func (t *Transport) WantsDiscoveryDestination() {
	if t == nil || t.destFilters == nil {
		return
	}
	requester := "discovery"
	if id := t.TransportIdentity(); id != nil {
		requester = string(id.Hash())
	}
	t.destFilters.wants("rnstransport.discovery.interface", requester)
}

// DestinationTypeName returns the Python destination_type_name result for a
// name hash: a registered or flagged name, else raw hex.
func (t *Transport) DestinationTypeName(nameHash []byte) string {
	if t == nil || t.destFilters == nil {
		return ""
	}
	return t.destFilters.typeName(nameHash)
}

// RegisterKnownDestinationType loads a known_destination_types entry.
func (t *Transport) RegisterKnownDestinationType(nameHash []byte, fullName string) {
	if t == nil || t.destFilters == nil {
		return
	}
	t.destFilters.registerKnown(nameHash, fullName)
}

// RegisterFlaggedDestinationType loads a flagged_destination_types entry.
func (t *Transport) RegisterFlaggedDestinationType(nameHash []byte, fullName string) {
	if t == nil || t.destFilters == nil {
		return
	}
	t.destFilters.registerFlagged(nameHash, fullName)
}

// LinkDistributionRPC mirrors Python Transport.link_distribution(), which is
// a stub returning {} upstream. Keep the same shape for RPC parity.
func (t *Transport) LinkDistributionRPC() map[string]any {
	return map[string]any{}
}
