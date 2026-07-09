// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sharedinstance

import (
	"fmt"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/transport"
)

// Mode describes how this process participates in the shared instance.
type Mode int

const (
	ModeDisabled Mode = iota
	ModeServer
	ModeClient
)

// Instance holds shared-instance state for a running node.
type Instance struct {
	Mode   Mode
	Server *interfaces.LocalServerInterface
	Client *interfaces.LocalClientInterface
	RPC    *RPCServer
}

// Hooks wires shared-instance clients into the transport stack.
type Hooks struct {
	RegisterInterface   func(name string, iface common.NetworkInterface) error
	UnregisterInterface func(name string)
	HandleInterface     func(iface common.NetworkInterface)
	OnClientAttach      func()
}

// Attach starts or joins a shared local instance when share_instance is
// enabled. It tries to bind the shared server first. On failure it connects
// as a client to an existing instance.
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

func (i *Instance) Close() {
	if i == nil {
		return
	}
	if i.RPC != nil {
		_ = i.RPC.Close()
	}
	if i.Server != nil {
		_ = i.Server.Stop()
	}
	if i.Client != nil {
		_ = i.Client.Stop()
	}
}

func (i *Instance) OwnsNetworkInterfaces() bool {
	if i == nil {
		return true
	}
	return i.Mode != ModeClient
}
