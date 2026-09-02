// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"fmt"

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
)

const (
	blackholeInfoAspectA = "info"
	blackholeInfoAspectB = "blackhole"
	blackholeListPath    = "/list"
)

// InitializeBlackholePublish registers rnstransport.info.blackhole with a
// /list request handler when publish_blackhole is enabled and this process
// owns the transport (not a shared-instance client).
func (t *Transport) InitializeBlackholePublish() error {
	if t == nil || t.config == nil || !t.config.PublishBlackhole {
		return nil
	}
	if t.ConnectedToSharedInstance() {
		return nil
	}
	if t.transportIdentity == nil {
		return fmt.Errorf("transport identity not initialized")
	}
	t.mutex.RLock()
	existing := t.blackholeDest
	t.mutex.RUnlock()
	if existing != nil {
		return nil
	}

	dest, err := destination.New(t.transportIdentity, destination.In, destination.Single, remoteManagementApp, t, blackholeInfoAspectA, blackholeInfoAspectB)
	if err != nil {
		return fmt.Errorf("blackhole publish destination: %w", err)
	}
	dest.AcceptsLinks(true)
	if err := dest.RegisterRequestHandlerAny(blackholeListPath, t.blackholeListHandler, destination.AllowAll, nil); err != nil {
		return fmt.Errorf("blackhole /list handler: %w", err)
	}

	t.RegisterDestination(dest.GetHash(), dest)
	t.mutex.Lock()
	t.blackholeDest = dest
	t.mgmtDestinations = append(t.mgmtDestinations, dest)
	t.mutex.Unlock()

	debug.Log(debug.DebugInfo, "Enabled blackhole publish",
		"destination", fmt.Sprintf("%x", dest.GetHash()))
	return nil
}

// BlackholePublishDestination returns the local blackhole /list destination.
func (t *Transport) BlackholePublishDestination() *destination.Destination {
	if t == nil {
		return nil
	}
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.blackholeDest
}

func (t *Transport) blackholeListHandler(_ string, _ []byte, _ []byte, _ []byte, _ *identity.Identity, _ int64) any {
	tab := t.BlackholeTable()
	if tab == nil {
		return []byte{}
	}
	packed, err := tab.EncodeForRequest()
	if err != nil {
		debug.Log(debug.DebugError, "Blackhole /list encode failed", "error", err)
		return nil
	}
	return packed
}
