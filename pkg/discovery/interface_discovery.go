// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"quad4/reticulum-go/pkg/announce"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/transport"
)

// InterfaceDiscovery listens for rnstransport interface discovery announces.
type InterfaceDiscovery struct {
	transport     *transport.Transport
	requiredValue int
	onDiscovered  func(*ReceivedAnnounceInfo)
	handler       *interfaceAnnounceHandler
}

// NewInterfaceDiscovery creates a discovery listener for interface announces.
func NewInterfaceDiscovery(tr *transport.Transport, requiredValue int, onDiscovered func(*ReceivedAnnounceInfo)) *InterfaceDiscovery {
	if requiredValue <= 0 {
		requiredValue = DefaultStampValue
	}
	return &InterfaceDiscovery{
		transport:     tr,
		requiredValue: requiredValue,
		onDiscovered:  onDiscovered,
	}
}

// Start registers the announce handler with transport.
func (d *InterfaceDiscovery) Start() {
	if d == nil || d.transport == nil || d.handler != nil {
		return
	}
	d.handler = &interfaceAnnounceHandler{
		requiredValue: d.requiredValue,
		onDiscovered:  d.onDiscovered,
	}
	d.transport.RegisterAnnounceHandler(d.handler)
}

// Stop unregisters the announce handler.
func (d *InterfaceDiscovery) Stop() {
	if d == nil || d.transport == nil || d.handler == nil {
		return
	}
	d.transport.UnregisterAnnounceHandler(d.handler)
	d.handler = nil
}

type interfaceAnnounceHandler struct {
	requiredValue int
	onDiscovered  func(*ReceivedAnnounceInfo)
}

func (h *interfaceAnnounceHandler) AspectFilter() []string {
	return []string{AppName + ".discovery.interface"}
}

func (h *interfaceAnnounceHandler) ReceivePathResponses() bool {
	return false
}

func (h *interfaceAnnounceHandler) ReceivedAnnounce(_ []byte, _ any, appData []byte, _ uint8) error {
	if len(appData) == 0 {
		return nil
	}
	info, err := ValidateAndDecode(appData, h.requiredValue, WorkblockExpandRounds)
	if err != nil {
		debug.Log(debug.DebugVerbose, "Ignored interface discovery announce", "error", err)
		return nil
	}
	if h.onDiscovered != nil {
		h.onDiscovered(info)
	}
	return nil
}

var _ announce.Handler = (*interfaceAnnounceHandler)(nil)
