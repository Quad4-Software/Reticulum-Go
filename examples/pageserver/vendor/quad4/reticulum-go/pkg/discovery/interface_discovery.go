// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"crypto/sha256"
	"sync"

	"quad4/reticulum-go/pkg/announce"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

const discoveryAnnounceCacheMax = 2048

// InterfaceDiscovery listens for rnstransport interface discovery announces.
type InterfaceDiscovery struct {
	transport     *transport.Transport
	requiredValue int
	onDiscovered  func(*ReceivedAnnounceInfo)
	isBlackholed  func([]byte) bool
	handler       *interfaceAnnounceHandler
	mu            sync.Mutex
}

// NewInterfaceDiscovery creates a discovery listener for interface announces.
// isBlackholed, when non-nil, drops announces whose transport id or announcer
// identity (Python network_id) is blackholed (RNS 1.4.2 list filtering,
// applied at receive time).
func NewInterfaceDiscovery(tr *transport.Transport, requiredValue int, onDiscovered func(*ReceivedAnnounceInfo)) *InterfaceDiscovery {
	return NewInterfaceDiscoveryWithBlackhole(tr, requiredValue, onDiscovered, nil)
}

// NewInterfaceDiscoveryWithBlackhole is like NewInterfaceDiscovery with an
// optional blackhole membership callback.
func NewInterfaceDiscoveryWithBlackhole(tr *transport.Transport, requiredValue int, onDiscovered func(*ReceivedAnnounceInfo), isBlackholed func([]byte) bool) *InterfaceDiscovery {
	if requiredValue <= 0 {
		requiredValue = DefaultStampValue
	}
	return &InterfaceDiscovery{
		transport:     tr,
		requiredValue: requiredValue,
		onDiscovered:  onDiscovered,
		isBlackholed:  isBlackholed,
	}
}

// Start registers the announce handler with transport.
func (d *InterfaceDiscovery) Start() {
	if d == nil || d.transport == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handler != nil {
		return
	}
	d.handler = &interfaceAnnounceHandler{
		requiredValue: d.requiredValue,
		onDiscovered:  d.onDiscovered,
		isBlackholed:  d.isBlackholed,
		networkID:     func() *identity.Identity { return d.transport.NetworkIdentity() },
		validCache:    make(map[string]*ReceivedAnnounceInfo),
		invalidCache:  make(map[string]struct{}),
	}
	d.transport.RegisterAnnounceHandler(d.handler)
}

// Stop unregisters the announce handler.
func (d *InterfaceDiscovery) Stop() {
	if d == nil || d.transport == nil {
		return
	}
	d.mu.Lock()
	handler := d.handler
	d.handler = nil
	d.mu.Unlock()
	if handler == nil {
		return
	}
	d.transport.UnregisterAnnounceHandler(handler)
}

type interfaceAnnounceHandler struct {
	requiredValue int
	onDiscovered  func(*ReceivedAnnounceInfo)
	isBlackholed  func([]byte) bool
	networkID     func() *identity.Identity

	mu           sync.Mutex
	validating   bool
	validCache   map[string]*ReceivedAnnounceInfo
	validOrder   []string
	invalidCache map[string]struct{}
	invalidOrder []string
}

func (h *interfaceAnnounceHandler) AspectFilter() []string {
	return []string{AppName + ".discovery.interface"}
}

func (h *interfaceAnnounceHandler) ReceivePathResponses() bool {
	return false
}

func (h *interfaceAnnounceHandler) ReceivedAnnounce(destHash []byte, announcedIdentity any, appData []byte, hops uint8) error {
	if len(appData) <= 1 {
		return nil
	}
	body := appData[1:]
	sum := sha256.Sum256(body)
	fullHash := string(sum[:])
	remoteHash := announcedIdentityHash(announcedIdentity)

	h.mu.Lock()
	if _, bad := h.invalidCache[fullHash]; bad {
		h.mu.Unlock()
		debug.Log(debug.DebugVerbose, "Ignored previously invalid discovery announce stamp")
		return nil
	}
	if cached, ok := h.validCache[fullHash]; ok {
		info := cached
		attachRemoteIdentity(info, remoteHash)
		info.Hops = hops
		cb := h.onDiscovered
		drop := discoveryInfoBlackholed(info, h.isBlackholed)
		h.mu.Unlock()
		if drop {
			debug.Log(debug.DebugVerbose, "Ignored blackholed interface discovery announce")
			return nil
		}
		if cb != nil {
			cb(info)
		}
		return nil
	}
	if h.validating {
		h.mu.Unlock()
		debug.Log(debug.DebugVerbose, "Dropping discovery announce while validating another stamp")
		return nil
	}
	h.validating = true
	h.mu.Unlock()

	var netID *identity.Identity
	if h.networkID != nil {
		netID = h.networkID()
	}
	info, err := ValidateAndDecodeWithIdentity(appData, h.requiredValue, WorkblockExpandRounds, netID)

	h.mu.Lock()
	h.validating = false
	if err != nil {
		h.rememberInvalidLocked(fullHash)
		h.mu.Unlock()
		debug.Log(debug.DebugVerbose, "Ignored interface discovery announce", "error", err)
		return nil
	}
	attachRemoteIdentity(info, remoteHash)
	info.Hops = hops
	h.rememberValidLocked(fullHash, info)
	cb := h.onDiscovered
	drop := discoveryInfoBlackholed(info, h.isBlackholed)
	h.mu.Unlock()

	if drop {
		debug.Log(debug.DebugVerbose, "Ignored blackholed interface discovery announce")
		return nil
	}
	if cb != nil {
		cb(info)
	}
	return nil
}

func announcedIdentityHash(announcedIdentity any) []byte {
	id, ok := announcedIdentity.(*identity.Identity)
	if !ok || id == nil {
		return nil
	}
	return id.Hash()
}

func attachRemoteIdentity(info *ReceivedAnnounceInfo, remoteHash []byte) {
	if info == nil || len(remoteHash) == 0 {
		return
	}
	if len(info.RemoteIdentity) == 0 {
		info.RemoteIdentity = append([]byte(nil), remoteHash...)
	}
}

// discoveryInfoBlackholed mirrors RNS 1.4.2 Discovery list filtering of
// transport_id and network_id (announcer identity hash). Applied at receive
// time so blackholed peers never enter discovery caches.
func discoveryInfoBlackholed(info *ReceivedAnnounceInfo, isBlackholed func([]byte) bool) bool {
	if info == nil || isBlackholed == nil {
		return false
	}
	if len(info.Info.TransportID) > 0 && isBlackholed(info.Info.TransportID) {
		return true
	}
	if len(info.RemoteIdentity) > 0 && isBlackholed(info.RemoteIdentity) {
		return true
	}
	return false
}

func (h *interfaceAnnounceHandler) rememberInvalidLocked(fullHash string) {
	if _, ok := h.invalidCache[fullHash]; ok {
		return
	}
	h.invalidCache[fullHash] = struct{}{}
	h.invalidOrder = append(h.invalidOrder, fullHash)
	for len(h.invalidOrder) > discoveryAnnounceCacheMax {
		old := h.invalidOrder[0]
		h.invalidOrder = h.invalidOrder[1:]
		delete(h.invalidCache, old)
	}
}

func (h *interfaceAnnounceHandler) rememberValidLocked(fullHash string, info *ReceivedAnnounceInfo) {
	if _, ok := h.validCache[fullHash]; ok {
		return
	}
	h.validCache[fullHash] = info
	h.validOrder = append(h.validOrder, fullHash)
	for len(h.validOrder) > discoveryAnnounceCacheMax {
		old := h.validOrder[0]
		h.validOrder = h.validOrder[1:]
		delete(h.validCache, old)
	}
}

var _ announce.Handler = (*interfaceAnnounceHandler)(nil)
