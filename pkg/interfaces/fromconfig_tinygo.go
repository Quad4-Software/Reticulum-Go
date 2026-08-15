// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build tinygo

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
	case "SerialInterface":
		baud := uint32(cfg.Bitrate) // #nosec G115
		if baud == 0 {
			baud = SerialDefaultBaud
		}
		iface, err = NewSerialInterface(name, cfg.Interface, baud, cfg.Enabled)
	case "RNodeInterface":
		baud := uint32(cfg.Bitrate) // #nosec G115
		if baud == 0 {
			baud = SerialDefaultBaud
		}
		serial, sErr := NewSerialInterface(name+"_serial", cfg.Interface, baud, cfg.Enabled)
		if sErr != nil {
			err = sErr
		} else {
			iface, err = NewRNodeInterface(
				name,
				serial,
				cfg.Frequency,
				uint32(cfg.Bandwidth), // #nosec G115
				cfg.SF,
				cfg.CR,
				cfg.TXPower,
			)
		}
	default:
		return nil, fmt.Errorf("unsupported interface type %q on TinyGo", cfg.Type)
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
