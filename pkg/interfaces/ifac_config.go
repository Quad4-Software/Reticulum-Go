// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"
	"strings"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/ifac"
)

// ApplyIFACFromConfig derives and attaches an Interface Access Code to iface
// when network_name, passphrase, ifac_netname, or ifac_netkey are set in cfg.
func ApplyIFACFromConfig(iface common.NetworkInterface, cfg *common.InterfaceConfig) error {
	if iface == nil || cfg == nil {
		return nil
	}
	netname := cfg.IFACNetname
	if netname == "" {
		netname = cfg.NetworkName
	}
	netkey := cfg.IFACNetkey
	if netkey == "" {
		netkey = cfg.Passphrase
	}
	if netname == "" && netkey == "" {
		return nil
	}
	size := cfg.IFACSize
	if size <= 0 {
		switch {
		case strings.EqualFold(cfg.Type, "SerialInterface"):
			size = serialDefaultIFACSize
		case strings.EqualFold(cfg.Type, "Modem73Interface"):
			size = modem73DefaultIFACSize
		case strings.EqualFold(cfg.Type, "SDRInterface"):
			size = sdrDefaultIFACSize
		default:
			size = ifac.DefaultSize
		}
	}
	id, err := ifac.New(size, netname, netkey)
	if err != nil {
		return fmt.Errorf("ifac for interface %q: %w", cfg.Name, err)
	}
	iface.SetIFAC(id)
	return nil
}
