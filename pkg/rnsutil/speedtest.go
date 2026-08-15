// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/transport"
)

const (
	// SpeedtestAppName is the destination app name for link speedtest.
	SpeedtestAppName = "speedtest"
	// SpeedtestAspect is the server aspect.
	SpeedtestAspect = "server"
	// DefaultSpeedtestPathTimeout is the default path/link wait for clients.
	DefaultSpeedtestPathTimeout = 30 * time.Second
)

// SpeedtestIdentityPath returns the default identity path under storage.
func SpeedtestIdentityPath(cfgStorage string) string {
	if cfgStorage == "" {
		return ""
	}
	return filepath.Join(cfgStorage, "identities", SpeedtestAppName)
}

// PrepareSpeedtestIdentity loads or creates the speedtest identity file.
func PrepareSpeedtestIdentity(path string) (*identity.Identity, error) {
	if path == "" {
		return nil, fmt.Errorf("empty identity path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return identity.FromFile(path)
	}
	id, err := identity.New()
	if err != nil {
		return nil, err
	}
	if err := id.ToFile(path); err != nil {
		return nil, err
	}
	return id, nil
}

// EstablishSpeedtestLink waits for a path and opens an outbound link to
// speedtest.server.
func EstablishSpeedtestLink(ctx context.Context, tr *transport.Transport, destHash []byte) (*link.Link, error) {
	if err := WaitPathWindow(ctx, tr, destHash); err != nil {
		return nil, fmt.Errorf("path: %w", err)
	}
	remote, err := identity.Recall(destHash)
	if err != nil {
		return nil, fmt.Errorf("recall: %w", err)
	}
	outDest, err := destination.New(remote, destination.Out, destination.Single, SpeedtestAppName, tr, SpeedtestAspect)
	if err != nil {
		return nil, err
	}
	l := link.NewLink(outDest, tr, nil, nil, nil)
	if err := activateOutboundLink(ctx, l); err != nil {
		return nil, fmt.Errorf("link: %w", err)
	}
	return l, nil
}
