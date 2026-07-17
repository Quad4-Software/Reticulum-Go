// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
)

// SetNetworkIdentity installs the network identity used for discovery encrypt
// and decrypt, and for rnstransport.network destinations. Only the first call
// succeeds, matching Python Transport.set_network_identity.
func (t *Transport) SetNetworkIdentity(id *identity.Identity) {
	if t == nil || id == nil {
		return
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if t.networkIdentity != nil {
		return
	}
	t.networkIdentity = id
}

// NetworkIdentity returns the configured network identity, or nil.
func (t *Transport) NetworkIdentity() *identity.Identity {
	if t == nil {
		return nil
	}
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.networkIdentity
}

// TransportIdentity returns the transport identity, or nil.
func (t *Transport) TransportIdentity() *identity.Identity {
	if t == nil {
		return nil
	}
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.transportIdentity
}

// HasNetworkIdentity reports whether a network identity is configured.
func (t *Transport) HasNetworkIdentity() bool {
	return t.NetworkIdentity() != nil
}

// InitializeNetworkIdentity loads or creates the identity at
// config.NetworkIdentityPath and registers network destinations when this
// process owns the transport (not a shared-instance client).
func (t *Transport) InitializeNetworkIdentity() error {
	if t == nil || t.config == nil {
		return nil
	}
	path := strings.TrimSpace(t.config.NetworkIdentityPath)
	if path == "" {
		return nil
	}
	path = expandUserPath(path)
	id, err := loadOrCreateNetworkIdentity(path)
	if err != nil {
		return err
	}
	t.SetNetworkIdentity(id)
	if t.config.ConnectedToSharedInstance {
		return nil
	}
	return t.registerNetworkDestinations(id)
}

func loadOrCreateNetworkIdentity(path string) (*identity.Identity, error) {
	if path == "" {
		return nil, fmt.Errorf("empty network identity path")
	}
	if _, err := os.Stat(path); err == nil {
		id, err := identity.FromFile(path)
		if err != nil {
			return nil, fmt.Errorf("load network identity %s: %w", path, err)
		}
		return id, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create network identity dir: %w", err)
	}
	id, err := identity.New()
	if err != nil {
		return nil, err
	}
	if err := id.ToFile(path); err != nil {
		return nil, fmt.Errorf("write network identity %s: %w", path, err)
	}
	debug.Log(debug.DebugInfo, "Created network identity", "path", path, "hash", fmt.Sprintf("%x", id.Hash()))
	return id, nil
}

func (t *Transport) registerNetworkDestinations(id *identity.Identity) error {
	if t == nil || id == nil {
		return nil
	}
	t.mutex.Lock()
	if t.networkDestination != nil {
		t.mutex.Unlock()
		return nil
	}
	t.mutex.Unlock()

	hashHex := hex.EncodeToString(id.Hash())
	instanceDest, err := destination.New(id, destination.In, destination.Single, "rnstransport", t, "network", "instance", hashHex)
	if err != nil {
		return fmt.Errorf("network instance destination: %w", err)
	}
	instanceDest.AcceptsLinks(false)

	networkDest, err := destination.New(id, destination.In, destination.Single, "rnstransport", t, "network")
	if err != nil {
		return fmt.Errorf("network destination: %w", err)
	}
	networkDest.AcceptsLinks(false)

	t.RegisterDestination(instanceDest.GetHash(), instanceDest)
	t.RegisterDestination(networkDest.GetHash(), networkDest)

	t.mutex.Lock()
	t.networkDestination = networkDest
	t.networkInstanceDest = instanceDest
	t.mutex.Unlock()

	debug.Log(debug.DebugInfo, "Registered network destinations",
		"network", fmt.Sprintf("%x", networkDest.GetHash()),
		"instance", fmt.Sprintf("%x", instanceDest.GetHash()))
	return nil
}

func expandUserPath(path string) string {
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
