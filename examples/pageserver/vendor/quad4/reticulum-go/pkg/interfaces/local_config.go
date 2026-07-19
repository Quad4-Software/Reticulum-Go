// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
)

const defaultLocalPort = 37428

// NewLocalFromConfig builds a LocalClientInterface or LocalServerInterface from
// an interface configuration block.
func NewLocalFromConfig(name string, cfg *common.InterfaceConfig, ctx *FromConfigContext) (Interface, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil interface config")
	}
	useUnix := common.SharedInstanceUsesUnix(cfg.SharedInstanceType)
	socketPath := cfg.InstanceName
	if socketPath == "" && useUnix {
		socketPath = "default"
	}
	port := cfg.Port
	if port == 0 {
		port = cfg.TargetPort
	}
	if port == 0 {
		port = defaultLocalPort
	}
	hub := backbone.Get()
	if ctx != nil && ctx.BackboneHub != nil {
		hub = ctx.BackboneHub
	}

	switch cfg.Type {
	case "LocalServerInterface":
		spawn := func(client *LocalClientInterface) {}
		if ctx != nil && ctx.SpawnLocal != nil {
			spawn = ctx.SpawnLocal
		} else if ctx != nil && ctx.RegisterPeer != nil && ctx.SetupPeer != nil {
			spawn = func(client *LocalClientInterface) {
				if err := ctx.RegisterPeer(client.GetName(), client); err != nil {
					return
				}
				ctx.SetupPeer(client)
			}
		}
		return NewLocalServerInterface(port, socketPath, useUnix, spawn, hub)
	case "LocalInterface":
		lc, err := NewLocalClientInterface(port, socketPath, useUnix, hub)
		if err != nil {
			return nil, err
		}
		lc.Name = name
		lc.Out = true
		return lc, nil
	default:
		return nil, fmt.Errorf("unsupported local interface type %q", cfg.Type)
	}
}
