// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build tinygo
// +build tinygo

package interfaces

import (
	"fmt"
	"net"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
)

const (
	HW_MTU                 = 1196
	DEFAULT_DISCOVERY_PORT = 29716
	DEFAULT_DATA_PORT      = 42671
	DEFAULT_GROUP_ID       = "reticulum"
	BITRATE_GUESS          = 10 * 1000 * 1000
)

type AutoInterface struct {
	BaseInterface
	groupID           []byte
	discoveryPort     int
	dataPort          int
	discoveryScope    string
	peers             map[string]*Peer
	linkLocalAddrs    []string
	adoptedInterfaces map[string]string
	interfaceServers  map[string]net.Conn
	multicastEchoes   map[string]time.Time
	mutex             sync.RWMutex
	outboundConn      net.Conn
}

type Peer struct {
	ifaceName string
	lastHeard time.Time
	conn      net.PacketConn
}

func NewAutoInterface(name string, config *common.InterfaceConfig) (*AutoInterface, error) {
	base := NewBaseInterface(name, common.IFTypeAuto, config.Enabled)
	base.Mode = common.IFModeFull
	base.In = true
	base.Out = false
	base.MTU = HW_MTU
	base.Bitrate = BITRATE_GUESS
	ai := &AutoInterface{
		BaseInterface: base,
		discoveryPort:     DEFAULT_DISCOVERY_PORT,
		dataPort:          DEFAULT_DATA_PORT,
		peers:             make(map[string]*Peer),
		linkLocalAddrs:    make([]string, 0),
		adoptedInterfaces: make(map[string]string),
		interfaceServers:  make(map[string]net.Conn),
		multicastEchoes:   make(map[string]time.Time),
	}

	if config.Port != 0 {
		ai.discoveryPort = config.Port
	}

	if config.GroupID != "" {
		ai.groupID = []byte(config.GroupID)
	} else {
		ai.groupID = []byte("reticulum")
	}

	return ai, nil
}

func (ai *AutoInterface) Start() error {
	// TinyGo doesn't support net.Interfaces() or multicast UDP
	return fmt.Errorf("AutoInterface not supported in TinyGo - requires interface enumeration and multicast UDP")
}

func (ai *AutoInterface) Send(data []byte, address string) error {
	return fmt.Errorf("Send not supported in TinyGo - requires UDP client connections")
}

func (ai *AutoInterface) Stop() error {
	ai.Mutex.Lock()
	defer ai.Mutex.Unlock()
	ai.Online = false
	return nil
}

func (ai *AutoInterface) SetWatchInterfaces(enabled bool) {}

func (ai *AutoInterface) RescanInterfaces() error {
	return nil
}
