// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"crypto/sha256"
	"sync"
)

// Destination-type filtering (RNS 1.6.0 [filter] semantics): announces for
// destination types are filtered by name_hash at ingress. blocked_destination
// types are always dropped; allowed_destination_types pass the
// required_only gate; client_destination_types tracks per-requester
// registration via wants/unwants calls.

type destinationFilters struct {
	mu      sync.RWMutex
	blocked map[string]bool
	allowed map[string]bool
	// name_hash -> {requester -> true}
	clients map[string]map[string]bool
	names   map[string]string // name_hash -> full name
}

func newDestinationFilters() *destinationFilters {
	return &destinationFilters{
		blocked: make(map[string]bool),
		allowed: make(map[string]bool),
		clients: make(map[string]map[string]bool),
		names:   make(map[string]string),
	}
}

func nameHashForType(fullName string) []byte {
	h := sha256.Sum256([]byte(fullName))
	return h[:10]
}

func nameHashKey(fullName string) string {
	return string(nameHashForType(fullName))
}

// BlockDestinationType permanently blocks a destination type.
func (f *destinationFilters) block(fullName string) {
	key := nameHashKey(fullName)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blocked[key] = true
	delete(f.clients, key)
	f.names[key] = fullName
}

// AllowDestinationType whitelists a destination type for required_only.
func (f *destinationFilters) allow(fullName string) {
	key := nameHashKey(fullName)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allowed[key] = true
	f.names[key] = fullName
}

// wants registers a requester for a destination type (Python
// _wants_destination_type; signature unchecked upstream).
func (f *destinationFilters) wants(fullName, requester string) bool {
	key := nameHashKey(fullName)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocked[key] {
		return false
	}
	if f.clients[key] == nil {
		f.clients[key] = make(map[string]bool)
	}
	f.clients[key][requester] = true
	f.names[key] = fullName
	return true
}

// unwants removes a requester's requirement (Python _unwants_destination_type).
func (f *destinationFilters) unwants(fullName, requester string) bool {
	key := nameHashKey(fullName)
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.clients[key]
	if !ok {
		return false
	}
	if _, was := entry[requester]; !was {
		return false
	}
	delete(entry, requester)
	if len(entry) == 0 {
		delete(f.clients, key)
	}
	return true
}

// unwantsAll removes every requirement for a requester (Python
// _unwants_all_destination_types).
func (f *destinationFilters) unwantsAll(requester string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for hash, entry := range f.clients {
		if _, was := entry[requester]; was {
			delete(entry, requester)
			if len(entry) == 0 {
				delete(f.clients, hash)
			}
		}
	}
	return true
}

// announceAllowed mirrors Python inbound announce gating for the type filters
// and required_only. hops is the wire hop count; localClient/interfaceToShared
// indicate a local-client/shared-instance interface (Python hops==1 passes).
func (f *destinationFilters) announceAllowed(nameHash []byte, hops int, requiredOnly bool, localClient bool) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.blocked[string(nameHash)] {
		return false
	}
	if !requiredOnly {
		return true
	}
	if f.allowed[string(nameHash)] {
		return true
	}
	if _, ok := f.clients[string(nameHash)]; ok {
		return true
	}
	if localClient && hops == 1 {
		return true
	}
	return false
}

func (f *destinationFilters) stats() (blocked, allowed int, clients int) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.blocked), len(f.allowed), len(f.clients)
}
