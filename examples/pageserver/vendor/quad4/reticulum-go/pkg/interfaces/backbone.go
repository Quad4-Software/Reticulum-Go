// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package interfaces

import (
	"fmt"
	"net"

	"quad4/reticulum-go/pkg/common"
)

// BackboneInterface is a high-throughput TCP server with HDLC framing.
type BackboneInterface struct {
	*TCPServerInterface
}

// NewBackboneInterface binds to the address of cfg.Interface, or to cfg.Address.
func NewBackboneInterface(name string, cfg *common.InterfaceConfig) (*BackboneInterface, error) {
	bindAddr := cfg.Address
	if bindAddr == "" {
		bindAddr = cfg.TargetHost
	}
	bindPort := cfg.Port
	if bindPort == 0 {
		bindPort = cfg.TargetPort
	}

	if cfg.Interface != "" {
		iface, err := net.InterfaceByName(cfg.Interface)
		if err != nil {
			return nil, fmt.Errorf("find interface %q: %w", cfg.Interface, err)
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("list addresses for %q: %w", cfg.Interface, err)
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP
			if cfg.PreferIPv6 {
				if ip.To4() == nil {
					bindAddr = ip.String()
					break
				}
			} else {
				if ip.To4() != nil {
					bindAddr = ip.String()
					break
				}
			}
		}
		if bindAddr == "" && len(addrs) > 0 {
			if ipnet, ok := addrs[0].(*net.IPNet); ok {
				bindAddr = ipnet.IP.String()
			}
		}
	}

	if bindPort == 0 {
		return nil, fmt.Errorf("no port for BackboneInterface %q", name)
	}

	ts, err := NewTCPServerInterface(name, bindAddr, bindPort, cfg.KISSFraming, cfg.I2PTunneled, cfg.PreferIPv6)
	if err != nil {
		return nil, err
	}

	ts.MTU = 1048576
	ts.Bitrate = 1000000000
	ts.Type = common.IFTypeBackbone

	return &BackboneInterface{TCPServerInterface: ts}, nil
}
