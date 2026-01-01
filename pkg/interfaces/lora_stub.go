// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
//go:build !tinygo

package interfaces

import (
	"fmt"
)

type LoRaInterface struct {
	BaseInterface
}

func NewLoRaInterface(name string, spi interface{}, cs, reset, dio0 interface{}, freq uint32, bw uint32, sf uint8, cr uint8, enabled bool) (*LoRaInterface, error) {
	return nil, fmt.Errorf("LoRaInterface is only supported on TinyGo targets currently")
}

func (li *LoRaInterface) Start() error {
	return fmt.Errorf("LoRaInterface is only supported on TinyGo targets currently")
}

func (li *LoRaInterface) Stop() error {
	return nil
}

func (li *LoRaInterface) Send(data []byte, address string) error {
	return fmt.Errorf("LoRaInterface is only supported on TinyGo targets currently")
}

