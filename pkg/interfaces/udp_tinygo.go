// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
//go:build tinygo
// +build tinygo

package interfaces

import (
	"fmt"
	"net"
	"sync"

	"quad4/reticulum-go/pkg/common"
)

type UDPInterface struct {
	BaseInterface
	conn       net.Conn
	addr       *net.UDPAddr
	targetAddr *net.UDPAddr
	readBuffer []byte
	done       chan struct{}
	stopOnce   sync.Once
}

func NewUDPInterface(name string, addr string, target string, enabled bool) (*UDPInterface, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	var targetAddr *net.UDPAddr
	if target != "" {
		targetAddr, err = net.ResolveUDPAddr("udp", target)
		if err != nil {
			return nil, err
		}
	}

	ui := &UDPInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeUDP, enabled),
		addr:          udpAddr,
		targetAddr:    targetAddr,
		readBuffer:    make([]byte, 1064),
		done:          make(chan struct{}),
	}

	ui.MTU = 1064

	return ui, nil
}

func NewUDPInterfaceWithRetries(name string, addr string, target string, enabled bool, maxReconnectTries int) (*UDPInterface, error) {
	_ = maxReconnectTries
	return NewUDPInterface(name, addr, target, enabled)
}

func (ui *UDPInterface) Start() error {
	// TinyGo doesn't support UDP servers, only clients
	return fmt.Errorf("UDPInterface not supported in TinyGo - UDP server functionality requires net.ListenUDP")
}

func (ui *UDPInterface) Send(data []byte, addr string) error {
	// TinyGo doesn't support UDP sending
	return fmt.Errorf("UDPInterface Send not supported in TinyGo - requires UDP client functionality")
}

func (ui *UDPInterface) Stop() error {
	ui.Mutex.Lock()
	defer ui.Mutex.Unlock()
	ui.Online = false
	return nil
}
