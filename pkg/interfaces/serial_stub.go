// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build !tinygo

package interfaces

import (
	"fmt"
)

type SerialInterface struct {
	BaseInterface
}

func NewSerialInterface(name string, portName string, baud uint32, enabled bool) (*SerialInterface, error) {
	return nil, fmt.Errorf("SerialInterface is only supported on TinyGo targets currently")
}

func (si *SerialInterface) Start() error {
	return fmt.Errorf("SerialInterface is only supported on TinyGo targets currently")
}

func (si *SerialInterface) Stop() error {
	return nil
}

func (si *SerialInterface) Send(data []byte, address string) error {
	return fmt.Errorf("SerialInterface is only supported on TinyGo targets currently")
}
