// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build !linux || js

package interfaces

import (
	"fmt"
	"math"
	"net"

	"quad4/reticulum-go/pkg/common"
)

// ParseVSOCKContextID converts a config integer to a vsock context ID.
func ParseVSOCKContextID(v int) (uint32, error) {
	if v < 0 {
		return 0, fmt.Errorf("vsock context ID must be non-negative, got %d", v)
	}
	if uint64(v) > math.MaxUint32 {
		return 0, fmt.Errorf("vsock context ID overflows uint32, got %d", v)
	}
	return uint32(v), nil
}

// VSOCKClientInterface is unavailable outside linux non-js builds.
type VSOCKClientInterface struct {
	BaseInterface
}

// VSOCKServerInterface is unavailable outside linux non-js builds.
type VSOCKServerInterface struct {
	BaseInterface
}

// NewVSOCKClientInterface returns an error on unsupported platforms.
func NewVSOCKClientInterface(name string, contextID uint32, port uint32, enabled bool) (*VSOCKClientInterface, error) {
	return nil, fmt.Errorf("VSOCKClientInterface is not supported on this platform")
}

// NewVSOCKClientInterfaceWithRetries returns an error on unsupported platforms.
func NewVSOCKClientInterfaceWithRetries(name string, contextID uint32, port uint32, enabled bool, maxTries int) (*VSOCKClientInterface, error) {
	return nil, fmt.Errorf("VSOCKClientInterface is not supported on this platform")
}

// NewVSOCKServerInterface returns an error on unsupported platforms.
func NewVSOCKServerInterface(name string, port uint32) (*VSOCKServerInterface, error) {
	return nil, fmt.Errorf("VSOCKServerInterface is not supported on this platform")
}

// SetConnectivityHooks is a stub.
func (vc *VSOCKClientInterface) SetConnectivityHooks(onDown, onUp func()) {}

// Start is a stub.
func (vc *VSOCKClientInterface) Start() error {
	return fmt.Errorf("VSOCKClientInterface is not supported on this platform")
}

// Stop is a stub.
func (vc *VSOCKClientInterface) Stop() error { return nil }

// ProcessOutgoing is a stub.
func (vc *VSOCKClientInterface) ProcessOutgoing([]byte) error {
	return fmt.Errorf("VSOCKClientInterface is not supported on this platform")
}

// Send is a stub.
func (vc *VSOCKClientInterface) Send([]byte, string) error {
	return fmt.Errorf("VSOCKClientInterface is not supported on this platform")
}

// ContextID is a stub.
func (vc *VSOCKClientInterface) ContextID() uint32 { return 0 }

// Port is a stub.
func (vc *VSOCKClientInterface) Port() uint32 { return 0 }

// SetListenContextID is a stub.
func (vs *VSOCKServerInterface) SetListenContextID(cid uint32) {}

// Start is a stub.
func (vs *VSOCKServerInterface) Start() error {
	return fmt.Errorf("VSOCKServerInterface is not supported on this platform")
}

// Stop is a stub.
func (vs *VSOCKServerInterface) Stop() error { return nil }

// SessionCount is a stub.
func (vs *VSOCKServerInterface) SessionCount() int { return 0 }

// ProcessOutgoing is a stub.
func (vs *VSOCKServerInterface) ProcessOutgoing([]byte) error {
	return fmt.Errorf("VSOCKServerInterface is not supported on this platform")
}

// Send is a stub.
func (vs *VSOCKServerInterface) Send([]byte, string) error {
	return fmt.Errorf("VSOCKServerInterface is not supported on this platform")
}

// ListenAddr is a stub.
func (vs *VSOCKServerInterface) ListenAddr() net.Addr { return nil }

// Port is a stub.
func (vs *VSOCKServerInterface) Port() uint32 { return 0 }

// GetType reports IFTypeVSOCK on stub builds.
func (vc *VSOCKClientInterface) GetType() common.InterfaceType { return common.IFTypeVSOCK }

// GetType reports IFTypeVSOCK on stub builds.
func (vs *VSOCKServerInterface) GetType() common.InterfaceType { return common.IFTypeVSOCK }
