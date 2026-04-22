// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
//go:build tinygo && tinygo.wasm

package interfaces

import "fmt"

// SerialInterface is not implemented on wasm/wasi: the generic machine.UART
// path imports host symbols (e.g. __tinygo_uart_configure) that wasmtime does
// not provide. Use -target=pico (or another bare-metal board) for UART/KISS.

type SerialInterface struct {
	BaseInterface
}

func NewSerialInterface(name string, portName string, baud uint32, enabled bool) (*SerialInterface, error) {
	return nil, fmt.Errorf("SerialInterface is not supported on wasm/wasi builds")
}

func (si *SerialInterface) Start() error {
	return fmt.Errorf("SerialInterface is not supported on wasm/wasi builds")
}

func (si *SerialInterface) Stop() error {
	return nil
}

func (si *SerialInterface) Send(data []byte, address string) error {
	return fmt.Errorf("SerialInterface is not supported on wasm/wasi builds")
}

func (si *SerialInterface) SendKISS(command byte, data []byte) error {
	return fmt.Errorf("SerialInterface is not supported on wasm/wasi builds")
}
