// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"encoding/binary"
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const (
	RNODE_CMD_DATA        = 0x00
	RNODE_CMD_FREQUENCY   = 0x01
	RNODE_CMD_BANDWIDTH   = 0x02
	RNODE_CMD_TXPOWER     = 0x03
	RNODE_CMD_SF          = 0x04
	RNODE_CMD_CR          = 0x05
	RNODE_CMD_RADIO_STATE = 0x06
	RNODE_CMD_RADIO_LOCK  = 0x07
	RNODE_CMD_DETECT      = 0x08
	RNODE_CMD_LEAVE       = 0x0A
	RNODE_CMD_ST_ALOCK    = 0x0B
	RNODE_CMD_LT_ALOCK    = 0x0C
	RNODE_CMD_READY       = 0x0F
	RNODE_CMD_STAT_RX     = 0x21
	RNODE_CMD_STAT_TX     = 0x22
	RNODE_CMD_STAT_RSSI   = 0x23
	RNODE_CMD_STAT_SNR    = 0x24
	RNODE_CMD_FW_VERSION  = 0x50
	RNODE_CMD_PLATFORM    = 0x48
	RNODE_CMD_MCU         = 0x49

	RNODE_DETECT_REQ  = 0x73
	RNODE_DETECT_RESP = 0x46

	RNODE_RSSI_OFFSET = 157
)

// RNodeInterface represents a Reticulum node interface.
type RNodeInterface struct {
	Interface
	frequency uint32
	bandwidth uint32
	sf        uint8
	cr        uint8
	txPower   uint8
	callback  common.PacketCallback

	rFrequency uint32
	rBandwidth uint32
	rTXPower   uint8
	rSF        uint8
	rCR        uint8
	rState     uint8
	rDetected  bool
	rMajVer    uint8
	rMinVer    uint8

	interfaceReady bool
	packetQueue    [][]byte
}

// NewRNodeInterface creates a new RNodeInterface.
func NewRNodeInterface(name string, underlying Interface, freq uint32, bw uint32, sf uint8, cr uint8, txPower uint8) (*RNodeInterface, error) {
	ri := &RNodeInterface{
		Interface: underlying,
		frequency: freq,
		bandwidth: bw,
		sf:        sf,
		cr:        cr,
		txPower:   txPower,
	}

	underlying.SetPacketCallback(ri.handleIncoming)
	return ri, nil
}

// SetPacketCallback sets the packet callback for the RNodeInterface.
func (ri *RNodeInterface) SetPacketCallback(cb common.PacketCallback) {
	ri.callback = cb
}

func (ri *RNodeInterface) handleIncoming(data []byte, ni common.NetworkInterface) {
	if len(data) < 1 {
		return
	}

	cmd := data[0]
	payload := data[1:]

	switch cmd {
	case RNODE_CMD_DATA:
		if ri.callback != nil {
			ri.callback(payload, ri)
		}
	case RNODE_CMD_READY:
		ri.processQueue()
	case RNODE_CMD_DETECT:
		if len(payload) >= 1 && payload[0] == RNODE_DETECT_RESP {
			ri.rDetected = true
		}
	case RNODE_CMD_FW_VERSION:
		if len(payload) >= 2 {
			ri.rMajVer = payload[0]
			ri.rMinVer = payload[1]
			debug.Log(debug.DebugInfo, "RNode firmware version", "name", ri.GetName(), "version", fmt.Sprintf("%d.%d", ri.rMajVer, ri.rMinVer))
		}
	case RNODE_CMD_FREQUENCY:
		if len(payload) >= 4 {
			ri.rFrequency = binary.BigEndian.Uint32(payload)
		}
	case RNODE_CMD_BANDWIDTH:
		if len(payload) >= 4 {
			ri.rBandwidth = binary.BigEndian.Uint32(payload)
		}
	case RNODE_CMD_TXPOWER:
		if len(payload) >= 1 {
			ri.rTXPower = payload[0]
		}
	case RNODE_CMD_SF:
		if len(payload) >= 1 {
			ri.rSF = payload[0]
		}
	case RNODE_CMD_CR:
		if len(payload) >= 1 {
			ri.rCR = payload[0]
		}
	case RNODE_CMD_RADIO_STATE:
		if len(payload) >= 1 {
			ri.rState = payload[0]
		}
	case RNODE_CMD_STAT_RSSI:
		if len(payload) >= 1 {
			rssi := int(payload[0]) - RNODE_RSSI_OFFSET
			debug.Log(debug.DebugVerbose, "RNode RSSI", "name", ri.GetName(), "rssi", rssi)
		}
	case RNODE_CMD_STAT_SNR:
		if len(payload) >= 1 {
			snr := float32(int8(payload[0])) * 0.25
			debug.Log(debug.DebugVerbose, "RNode SNR", "name", ri.GetName(), "snr", snr)
		}
	default:
		debug.Log(debug.DebugAll, "RNode received command", "cmd", fmt.Sprintf("0x%02x", cmd), "len", len(payload))
	}
}

func (ri *RNodeInterface) processQueue() {
	ri.interfaceReady = true
	if len(ri.packetQueue) > 0 {
		packet := ri.packetQueue[0]
		ri.packetQueue = ri.packetQueue[1:]
		_ = ri.Send(packet, "")
	}
}

// Start initializes the RNodeInterface and configures device parameters.
func (ri *RNodeInterface) Start() error {
	err := ri.Interface.Start()
	if err != nil {
		return err
	}

	time.Sleep(2 * time.Second)

	if err := ri.detect(); err != nil {
		return err
	}

	debug.Log(debug.DebugInfo, "Initializing RNode...", "name", ri.GetName())

	if ri.frequency != 0 {
		freqBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(freqBytes, ri.frequency)
		if err := ri.sendRNodeCommand(RNODE_CMD_FREQUENCY, freqBytes); err != nil {
			return err
		}
	}

	if ri.bandwidth != 0 {
		bwBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(bwBytes, ri.bandwidth)
		if err := ri.sendRNodeCommand(RNODE_CMD_BANDWIDTH, bwBytes); err != nil {
			return err
		}
	}

	if ri.sf != 0 {
		if err := ri.sendRNodeCommand(RNODE_CMD_SF, []byte{ri.sf}); err != nil {
			return err
		}
	}

	if ri.cr != 0 {
		if err := ri.sendRNodeCommand(RNODE_CMD_CR, []byte{ri.cr}); err != nil {
			return err
		}
	}

	if ri.txPower != 0 {
		if err := ri.sendRNodeCommand(RNODE_CMD_TXPOWER, []byte{ri.txPower}); err != nil {
			return err
		}
	}

	if err := ri.sendRNodeCommand(RNODE_CMD_RADIO_STATE, []byte{0x01}); err != nil {
		return err
	}

	ri.interfaceReady = true

	debug.Log(debug.DebugInfo, "RNode initialized", "name", ri.GetName())
	return nil
}

// detect attempts to detect the RNode device and obtain firmware version.
func (ri *RNodeInterface) detect() error {
	detectCmd := []byte{RNODE_DETECT_REQ}
	if err := ri.sendRNodeCommand(RNODE_CMD_DETECT, detectCmd); err != nil {
		return err
	}

	start := time.Now()
	for !ri.rDetected {
		if time.Since(start) > 2*time.Second {
			debug.Log(debug.DebugError, "RNode detection timed out", "name", ri.GetName())
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := ri.sendRNodeCommand(RNODE_CMD_FW_VERSION, []byte{0x00}); err != nil {
		return err
	}

	return nil
}

// sendRNodeCommand sends a command to the RNode device.
func (ri *RNodeInterface) sendRNodeCommand(cmd byte, data []byte) error {
	if kissInterface, ok := ri.Interface.(interface {
		SendKISS(byte, []byte) error
	}); ok {
		return kissInterface.SendKISS(cmd, data)
	}

	frame := make([]byte, 0, len(data)+1)
	frame = append(frame, cmd)
	frame = append(frame, data...)
	return ri.Interface.Send(frame, "")
}

// Send transmits data using the underlying interface.
func (ri *RNodeInterface) Send(data []byte, addr string) error {
	if !ri.interfaceReady {
		ri.packetQueue = append(ri.packetQueue, data)
		return nil
	}
	return ri.Interface.Send(data, addr)
}
