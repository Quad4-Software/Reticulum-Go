// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build js

package interfaces

import (
	"fmt"
	"runtime"
	"time"

	"quad4/reticulum-go/pkg/common"
)

// RNodeOptions is unavailable on this platform.
type RNodeOptions struct {
	Port                  string
	Frequency             int64
	Bandwidth             int
	TXPower               int
	SF                    int
	CR                    int
	FlowControl           bool
	IDInterval            time.Duration
	Callsign              string
	STAirTimeLock         *float64
	LTAirTimeLock         *float64
	MaxReconnectTries     int
	PanicOnInterfaceError bool
}

// RNodeInterface is unavailable on this platform.
type RNodeInterface struct {
	BaseInterface
}

// NewRNodeInterface returns an unsupported-platform error.
func NewRNodeInterface(string, bool, RNodeOptions) (*RNodeInterface, error) {
	return nil, fmt.Errorf("RNodeInterface is not available on %s/%s", runtime.GOOS, runtime.GOARCH)
}

// RNodeMultiOptions is unavailable on this platform.
type RNodeMultiOptions struct {
	RNodeOptions
	SubInterfaces  map[string]*common.InterfaceConfig
	RegisterPeer   func(name string, peer common.NetworkInterface) error
	UnregisterPeer func(name string)
	SetupPeer      func(peer common.NetworkInterface)
}

// RNodeMultiInterface is unavailable on this platform.
type RNodeMultiInterface struct {
	BaseInterface
}

// NewRNodeMultiInterface returns an unsupported-platform error.
func NewRNodeMultiInterface(string, bool, RNodeMultiOptions) (*RNodeMultiInterface, error) {
	return nil, fmt.Errorf("RNodeMultiInterface is not available on %s/%s", runtime.GOOS, runtime.GOARCH)
}
