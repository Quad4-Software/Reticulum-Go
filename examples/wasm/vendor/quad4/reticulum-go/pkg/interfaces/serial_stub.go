// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build js || !(linux || darwin || freebsd || openbsd || windows)

package interfaces

import (
	"fmt"
	"runtime"
	"time"

	"quad4/reticulum-go/pkg/common"
)

const serialDefaultIFACSize = 8

// SerialInterface is unavailable on this platform.
type SerialInterface struct {
	BaseInterface
}

// NewSerialInterface returns an error on unsupported platforms.
func NewSerialInterface(name string, enabled bool, opts SerialOptions) (*SerialInterface, error) {
	return nil, fmt.Errorf("SerialInterface is not available on %s/%s", runtime.GOOS, runtime.GOARCH)
}

// SerialOptions is a stub for unsupported platforms.
type SerialOptions struct {
	Device            string
	Speed             int
	DataBits          int
	Parity            string
	StopBits          int
	RTSCTS            bool
	DSRDTR            bool
	XONXOFF           bool
	FrameIdle         time.Duration
	ReconnectDelay    time.Duration
	MaxReconnectTries int
	MTU               int
	Bitrate           int64
}

func (si *SerialInterface) GetType() common.InterfaceType { return common.IFTypeSerial }
