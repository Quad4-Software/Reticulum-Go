// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package interfaces

import (
	"errors"
	"fmt"

	"quad4/reticulum-go/pkg/common"
)

// NewFromConfig constructs a logical interface from a loaded [common.InterfaceConfig].
func NewFromConfig(name string, cfg *common.InterfaceConfig) (Interface, error) {
	return NewFromConfigWithContext(name, cfg, nil)
}

// NewFromConfigWithContext constructs an interface using optional runtime context.
func NewFromConfigWithContext(name string, cfg *common.InterfaceConfig, ctx *FromConfigContext) (Interface, error) {
	if cfg == nil {
		return nil, errors.New("nil interface config")
	}
	var (
		iface Interface
		err   error
	)
	switch cfg.Type {
	case "TCPClientInterface":
		iface, err = NewTCPClientInterface(
			name,
			cfg.TargetHost,
			cfg.TargetPort,
			cfg.KISSFraming,
			cfg.I2PTunneled,
			cfg.Enabled,
		)
	case "UDPInterface":
		iface, err = NewUDPInterface(
			name,
			cfg.Address,
			cfg.TargetHost,
			cfg.Enabled,
		)
	case "AutoInterface":
		iface, err = NewAutoInterface(name, cfg)
	case "BackboneInterface":
		iface, err = NewBackboneInterface(name, cfg)
	case "WebSocketInterface":
		wsURL := cfg.Address
		if wsURL == "" {
			wsURL = cfg.TargetHost
		}
		iface, err = NewWebSocketInterface(name, wsURL, cfg.Enabled)
	case "TCPServerInterface":
		iface, err = NewTCPServerInterface(
			name,
			cfg.Address,
			cfg.Port,
			cfg.KISSFraming,
			cfg.I2PTunneled,
			cfg.PreferIPv6,
		)
	case "I2PInterface":
		parent, perr := NewI2PInterface(name, cfg, ctx)
		if perr != nil {
			return nil, perr
		}
		for _, peerAddr := range cfg.I2PPeers {
			peerName := name + " to " + peerAddr
			maxTries := cfg.MaxReconnTries
			peer := NewI2PInterfacePeer(parent, peerName, peerAddr, maxTries)
			parent.registerSpawnedPeer(peer)
		}
		iface = parent
	default:
		return nil, fmt.Errorf("unsupported interface type %q", cfg.Type)
	}
	if err != nil {
		return nil, err
	}
	ni, ok := iface.(common.NetworkInterface)
	if !ok {
		return nil, fmt.Errorf("interface %q does not implement common.NetworkInterface", name)
	}
	if err := ApplyIFACFromConfig(ni, cfg); err != nil {
		return nil, err
	}
	return iface, nil
}
