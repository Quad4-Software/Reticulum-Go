// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sharedinstance

import (
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
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
