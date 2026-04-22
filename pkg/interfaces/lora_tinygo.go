// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
//go:build tinygo

package interfaces

import (
	"fmt"
	"machine"
	"sync"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
)

const (
	REG_FIFO                 = 0x00
	REG_OP_MODE              = 0x01
	REG_FRF_MSB              = 0x06
	REG_FRF_MID              = 0x07
	REG_FRF_LSB              = 0x08
	REG_PA_CONFIG            = 0x09
	REG_FIFO_ADDR_PTR        = 0x0D
	REG_FIFO_TX_BASE_ADDR    = 0x0E
	REG_FIFO_RX_BASE_ADDR    = 0x0F
	REG_FIFO_RX_CURRENT_ADDR = 0x10
	REG_IRQ_FLAGS            = 0x12
	REG_RX_NB_BYTES          = 0x13
	REG_MODEM_CONFIG_1       = 0x1D
	REG_MODEM_CONFIG_2       = 0x1E
	REG_PREAMBLE_MSB         = 0x20
	REG_PREAMBLE_LSB         = 0x21
	REG_PAYLOAD_LENGTH       = 0x22
	REG_MODEM_CONFIG_3       = 0x26
	REG_RSSI_WIDEBAND        = 0x2C
	REG_DETECTION_OPTIMIZE   = 0x31
	REG_DETECTION_THRESHOLD  = 0x37
	REG_SYNC_WORD            = 0x39
	REG_DIO_MAPPING_1        = 0x40
	REG_VERSION              = 0x42

	MODE_LONG_RANGE_MODE = 0x80
	MODE_SLEEP           = 0x00
	MODE_STDBY           = 0x01
	MODE_TX              = 0x03
	MODE_RX_CONTINUOUS   = 0x05

	IRQ_RX_DONE_MASK           = 0x40
	IRQ_PAYLOAD_CRC_ERROR_MASK = 0x20
	IRQ_TX_DONE_MASK           = 0x08

	MAX_PKT_LENGTH = 255
)

// LoRaInterface provides a TinyGo SPI-based LoRa interface for SX127x.
type LoRaInterface struct {
	BaseInterface
	spi      machine.SPI
	cs       machine.Pin
	reset    machine.Pin
	dio0     machine.Pin
	freq     uint32
	bw       uint32
	sf       uint8
	cr       uint8
	txPower  uint8
	done     chan struct{}
	stopOnce sync.Once
}

// NewLoRaInterface initializes a new LoRaInterface.
func NewLoRaInterface(name string, spi machine.SPI, cs, reset, dio0 machine.Pin, freq uint32, bw uint32, sf uint8, cr uint8, enabled bool) (*LoRaInterface, error) {
	li := &LoRaInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeNone, enabled),
		spi:           spi,
		cs:            cs,
		reset:         reset,
		dio0:          dio0,
		freq:          freq,
		bw:            bw,
		sf:            sf,
		cr:            cr,
		txPower:       17,
		done:          make(chan struct{}),
	}

	li.MTU = MAX_PKT_LENGTH
	li.Bitrate = int64(bw * uint32(sf) / (1 << (sf - 1)))

	if enabled {
		err := li.Start()
		if err != nil {
			return nil, err
		}
	}

	return li, nil
}

// Start configures and brings the LoRaInterface online.
func (li *LoRaInterface) Start() error {
	li.Mutex.Lock()
	defer li.Mutex.Unlock()

	if li.Online {
		return nil
	}

	li.cs.Configure(machine.PinConfig{Mode: machine.PinOutput})
	li.cs.High()
	li.reset.Configure(machine.PinConfig{Mode: machine.PinOutput})
	li.dio0.Configure(machine.PinConfig{Mode: machine.PinInput})

	li.reset.Low()
	time.Sleep(10 * time.Millisecond)
	li.reset.High()
	time.Sleep(10 * time.Millisecond)

	version := li.readReg(REG_VERSION)
	if version != 0x12 {
		return fmt.Errorf("LoRa chip not found, version: 0x%02x", version)
	}

	li.writeReg(REG_OP_MODE, MODE_LONG_RANGE_MODE|MODE_SLEEP)
	time.Sleep(10 * time.Millisecond)

	frf := uint64(li.freq) << 19 / 32000000
	li.writeReg(REG_FRF_MSB, uint8(frf>>16))
	li.writeReg(REG_FRF_MID, uint8(frf>>8))
	li.writeReg(REG_FRF_LSB, uint8(frf))

	li.writeReg(REG_FIFO_TX_BASE_ADDR, 0)
	li.writeReg(REG_FIFO_RX_BASE_ADDR, 0)
	li.writeReg(0x0C, 0x23)
	li.writeReg(REG_MODEM_CONFIG_3, 0x04)
	li.writeReg(REG_PA_CONFIG, 0x80|(li.txPower-2))

	var bwVal uint8
	switch li.bw {
	case 125000:
		bwVal = 7
	case 250000:
		bwVal = 8
	case 500000:
		bwVal = 9
	default:
		bwVal = 7
	}
	li.writeReg(REG_MODEM_CONFIG_1, (bwVal<<4)|(li.cr-4)<<1|0x00)
	li.writeReg(REG_MODEM_CONFIG_2, (li.sf<<4)|0x04)

	li.writeReg(REG_SYNC_WORD, 0x12)
	li.writeReg(REG_OP_MODE, MODE_LONG_RANGE_MODE|MODE_STDBY)
	li.writeReg(REG_OP_MODE, MODE_LONG_RANGE_MODE|MODE_RX_CONTINUOUS)

	li.Online = true
	li.Enabled = true

	go li.readLoop()

	return nil
}

// readReg reads a byte from the given register.
func (li *LoRaInterface) readReg(reg uint8) uint8 {
	li.cs.Low()
	li.spi.Transfer(reg & 0x7F)
	val, _ := li.spi.Transfer(0)
	li.cs.High()
	return val
}

// writeReg writes a byte to the given register.
func (li *LoRaInterface) writeReg(reg uint8, val uint8) {
	li.cs.Low()
	li.spi.Transfer(reg | 0x80)
	li.spi.Transfer(val)
	li.cs.High()
}

// readLoop polls the radio for received packets and dispatches them.
func (li *LoRaInterface) readLoop() {
	for {
		li.Mutex.RLock()
		online := li.Online
		done := li.done
		li.Mutex.RUnlock()

		if !online {
			return
		}

		select {
		case <-done:
			return
		default:
		}

		irq := li.readReg(REG_IRQ_FLAGS)
		if irq&IRQ_RX_DONE_MASK != 0 {
			li.writeReg(REG_IRQ_FLAGS, IRQ_RX_DONE_MASK)

			if irq&IRQ_PAYLOAD_CRC_ERROR_MASK == 0 {
				currentAddr := li.readReg(REG_FIFO_RX_CURRENT_ADDR)
				li.writeReg(REG_FIFO_ADDR_PTR, currentAddr)
				count := li.readReg(REG_RX_NB_BYTES)

				packet := make([]byte, count)
				li.cs.Low()
				li.spi.Transfer(REG_FIFO)
				for i := uint8(0); i < count; i++ {
					packet[i], _ = li.spi.Transfer(0)
				}
				li.cs.High()

				li.ProcessIncoming(packet)
			}
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// Send transmits a packet over LoRa.
func (li *LoRaInterface) Send(data []byte, address string) error {
	return li.ProcessOutgoing(data)
}

// ProcessOutgoing encodes and sends a packet.
func (li *LoRaInterface) ProcessOutgoing(data []byte) error {
	li.Mutex.Lock()
	defer li.Mutex.Unlock()

	if !li.Online {
		return fmt.Errorf("interface offline")
	}

	if len(data) > MAX_PKT_LENGTH {
		return fmt.Errorf("packet too long for LoRa: %d", len(data))
	}

	li.writeReg(REG_OP_MODE, MODE_LONG_RANGE_MODE|MODE_STDBY)
	li.writeReg(REG_FIFO_ADDR_PTR, 0)

	li.cs.Low()
	li.spi.Transfer(REG_FIFO | 0x80)
	for _, b := range data {
		li.spi.Transfer(b)
	}
	li.cs.High()

	li.writeReg(REG_PAYLOAD_LENGTH, uint8(len(data)))
	li.writeReg(REG_OP_MODE, MODE_LONG_RANGE_MODE|MODE_TX)

	start := time.Now()
	for {
		if li.readReg(REG_IRQ_FLAGS)&IRQ_TX_DONE_MASK != 0 {
			li.writeReg(REG_IRQ_FLAGS, IRQ_TX_DONE_MASK)
			break
		}
		if time.Since(start) > 2*time.Second {
			debug.Log(debug.DebugError, "LoRa TX timeout")
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	li.writeReg(REG_OP_MODE, MODE_LONG_RANGE_MODE|MODE_RX_CONTINUOUS)

	li.TxBytes += uint64(len(data))
	li.lastTx = time.Now()

	return nil
}

// Stop disables the LoRaInterface.
func (li *LoRaInterface) Stop() error {
	li.Mutex.Lock()
	li.Online = false
	li.Enabled = false
	li.writeReg(REG_OP_MODE, MODE_LONG_RANGE_MODE|MODE_SLEEP)
	li.Mutex.Unlock()

	li.stopOnce.Do(func() {
		if li.done != nil {
			close(li.done)
		}
	})

	return nil
}
