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
	if cfg == nil {
		return nil, errors.New("nil interface config")
	}
	switch cfg.Type {
	case "TCPClientInterface":
		return NewTCPClientInterface(
			name,
			cfg.TargetHost,
			cfg.TargetPort,
			cfg.KISSFraming,
			cfg.I2PTunneled,
			cfg.Enabled,
		)
	case "UDPInterface":
		return NewUDPInterface(
			name,
			cfg.Address,
			cfg.TargetHost,
			cfg.Enabled,
		)
	case "AutoInterface":
		return NewAutoInterface(name, cfg)
	case "BackboneInterface":
		return NewBackboneInterface(name, cfg)
	case "WebSocketInterface":
		wsURL := cfg.Address
		if wsURL == "" {
			wsURL = cfg.TargetHost
		}
		return NewWebSocketInterface(name, wsURL, cfg.Enabled)
	case "TCPServerInterface":
		return NewTCPServerInterface(
			name,
			cfg.Address,
			cfg.Port,
			cfg.KISSFraming,
			cfg.I2PTunneled,
			cfg.PreferIPv6,
		)
	default:
		return nil, fmt.Errorf("unsupported interface type %q", cfg.Type)
	}
}
