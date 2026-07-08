// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !tinygo

package sharedinstance

import (
	"fmt"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/transport"
)

// Attach implements Python Reticulum.__start_local_interface.
func Attach(cfg *common.ReticulumConfig, tr *transport.Transport, hooks Hooks) (*Instance, error) {
	if cfg == nil || !cfg.ShareInstance {
		return &Instance{Mode: ModeDisabled}, nil
	}
	useUnix := cfg.SharedInstanceType == common.SharedInstanceUnix
	socketPath := cfg.InstanceName
	if socketPath == "" && useUnix {
		socketPath = "default"
	}

	inst := &Instance{}
	spawn := func(client *interfaces.LocalClientInterface) {
		if hooks.RegisterInterface == nil || hooks.HandleInterface == nil {
			return
		}
		if err := hooks.RegisterInterface(client.GetName(), client); err != nil {
			debug.Log(debug.DebugCritical, "Failed to register spawned local client", "error", err)
			return
		}
		hooks.HandleInterface(client)
	}

	server, err := interfaces.NewLocalServerInterface(cfg.SharedInstancePort, socketPath, useUnix, spawn, backbone.Get())
	if err != nil {
		return nil, err
	}
	if err := server.Start(); err == nil {
		inst.Mode = ModeServer
		inst.Server = server
		if hooks.RegisterInterface != nil {
			if err := hooks.RegisterInterface(server.GetName(), server); err != nil {
				_ = server.Stop()
				return nil, fmt.Errorf("register shared server: %w", err)
			}
		}
		rpc, err := StartRPCServer(cfg, tr)
		if err != nil {
			_ = server.Stop()
			return nil, err
		}
		inst.RPC = rpc
		debug.Log(debug.DebugInfo, "Started shared instance server", "port", cfg.SharedInstancePort)
		return inst, nil
	}

	client, err := interfaces.NewLocalClientInterface(cfg.SharedInstancePort, socketPath, useUnix, backbone.Get())
	if err != nil {
		return nil, err
	}
	client.SetDisconnectHooks(
		func() { tr.SetConnectedToSharedInstance(true) },
		func() { tr.SetConnectedToSharedInstance(true) },
	)
	if err := client.Start(); err != nil {
		return nil, fmt.Errorf("connect to local shared instance: %w", err)
	}
	tr.SetConnectedToSharedInstance(true)
	cfg.ConnectedToSharedInstance = true
	inst.Mode = ModeClient
	inst.Client = client
	if hooks.RegisterInterface != nil {
		if err := hooks.RegisterInterface(client.GetName(), client); err != nil {
			_ = client.Stop()
			return nil, err
		}
	}
	if hooks.HandleInterface != nil {
		hooks.HandleInterface(client)
	}
	if hooks.OnClientAttach != nil {
		hooks.OnClientAttach()
	}
	debug.Log(debug.DebugInfo, "Connected to existing local shared instance")
	return inst, nil
}
