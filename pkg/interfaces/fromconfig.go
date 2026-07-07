// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package interfaces

import (
	"errors"
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/backbone"
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
	case "UDPInterface":
		target := cfg.TargetAddress
		if target == "" {
			target = cfg.TargetHost
		}
		iface, err = NewUDPInterfaceWithRetries(
			name,
			cfg.Address,
			target,
			cfg.Enabled,
			cfg.MaxReconnTries,
		)
	case "AutoInterface":
		iface, err = NewAutoInterface(name, cfg)
		if err == nil {
			if auto, ok := iface.(*AutoInterface); ok && ctx != nil && ctx.WatchInterfaces {
				auto.SetWatchInterfaces(true)
			}
		}
	case "TCPClientInterface":
		iface, err = NewTCPClientInterfaceWithRetries(
			name,
			cfg.TargetHost,
			cfg.TargetPort,
			cfg.KISSFraming,
			cfg.I2PTunneled,
			cfg.Enabled,
			cfg.MaxReconnTries,
		)
		if err == nil {
			if tc, ok := iface.(*TCPClientInterface); ok && ctx != nil && ctx.SynthesizeTunnel != nil {
				tc.SetTunnelSynth(ctx.SynthesizeTunnel)
			}
		}
	case "BackboneInterface", "BackboneClientInterface":
		var hub *backbone.Hub
		var spawn func(*BackboneClientInterface)
		if ctx != nil {
			hub = ctx.BackboneHub
			spawn = ctx.SpawnBackbone
		}
		iface, err = NewBackboneFromConfig(name, cfg, hub, spawn)
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
			peer := NewI2PInterfacePeer(parent, peerName, peerAddr, maxTries, cfg)
			parent.registerSpawnedPeer(peer)
		}
		iface = parent
	case "PipeInterface":
		delay := time.Duration(cfg.RespawnDelay) * time.Second
		panicOnErr := ctx != nil && ctx.PanicOnInterfaceError
		iface, err = NewPipeInterface(name, cfg.Command, cfg.Enabled, delay, panicOnErr)
	case "LocalInterface", "LocalServerInterface":
		iface, err = NewLocalFromConfig(name, cfg, ctx)
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
