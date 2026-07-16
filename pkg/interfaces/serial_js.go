// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build js

package interfaces

import (
	"errors"
	"time"

	"quad4/reticulum-go/pkg/common"
)

const serialDefaultIFACSize = 8

// SerialInterface is unavailable under js/wasm.
type SerialInterface struct {
	BaseInterface
}

// NewSerialInterface returns an error on js/wasm.
func NewSerialInterface(name string, enabled bool, opts SerialOptions) (*SerialInterface, error) {
	return nil, errors.New("SerialInterface is not available on js/wasm")
}

// SerialOptions is a stub for js builds.
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
