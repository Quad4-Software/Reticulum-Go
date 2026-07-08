// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build tinygo && !tinygo.wasm

package interfaces

import (
	"fmt"
	"machine"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const (
	SERIAL_DEFAULT_BAUD = 115200
	SERIAL_MTU          = 1500
)

// serialTransport is the subset of machine.Serial used for I/O. Configure is
// not part of this interface: some targets use Configure() error, others
// Configure() with no return (e.g. WASI generic UART).
type serialTransport interface {
	Buffered() int
	ReadByte() (byte, error)
	Write(data []byte) (n int, err error)
}

func configureSerialPort(u any, cfg machine.UARTConfig) error {
	switch t := u.(type) {
	case interface {
		Configure(machine.UARTConfig) error
	}:
		return t.Configure(cfg)
	case interface {
		Configure(machine.UARTConfig)
	}:
		t.Configure(cfg)
		return nil
	default:
		return nil
	}
}

// SerialInterface implements a serial interface using TinyGo UART.
type SerialInterface struct {
	BaseInterface
	uart     serialTransport
	baud     uint32
	done     chan struct{}
	stopOnce sync.Once
}

// NewSerialInterface creates and initializes a new SerialInterface.
func NewSerialInterface(name string, portName string, baud uint32, enabled bool) (*SerialInterface, error) {
	if baud == 0 {
		baud = SERIAL_DEFAULT_BAUD
	}

	uart, err := getUART(portName)
	if err != nil {
		return nil, err
	}

	si := &SerialInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeSerial, enabled),
		uart:          uart,
		baud:          baud,
		done:          make(chan struct{}),
	}

	si.MTU = SERIAL_MTU
	si.Bitrate = int64(baud)

	if enabled {
		err := si.Start()
		if err != nil {
			return nil, err
		}
	}

	return si, nil
}

// getUART returns the default serial port (USB CDC, hardware UART, or WASI stub UART).
func getUART(name string) (serialTransport, error) {
	_ = name
	return machine.Serial, nil
}

// Start enables the serial interface and starts the read loop.
func (si *SerialInterface) Start() error {
	si.Mutex.Lock()
	defer si.Mutex.Unlock()

	if si.Online {
		return nil
	}

	if err := configureSerialPort(si.uart, machine.UARTConfig{
		BaudRate: si.baud,
	}); err != nil {
		return err
	}

	si.Online = true
	si.Enabled = true

	go si.readLoop()

	return nil
}

// Stop disables the serial interface.
func (si *SerialInterface) Stop() error {
	si.Mutex.Lock()
	si.Online = false
	si.Enabled = false
	si.Mutex.Unlock()

	si.stopOnce.Do(func() {
		if si.done != nil {
			close(si.done)
		}
	})

	return nil
}

// readLoop reads and processes frames from the UART, handling KISS framing.
func (si *SerialInterface) readLoop() {
	dataBuffer := make([]byte, 0, si.MTU)
	inFrame := false
	escape := false

	for {
		si.Mutex.RLock()
		online := si.Online
		done := si.done
		si.Mutex.RUnlock()

		if !online {
			return
		}

		select {
		case <-done:
			return
		default:
		}

		for si.uart.Buffered() > 0 {
			b, err := si.uart.ReadByte()
			if err != nil {
				debug.Log(debug.DebugError, "Serial read error", "name", si.Name, "error", err)
				time.Sleep(100 * time.Millisecond)
				break
			}

			if b == KISSFend {
				if inFrame && len(dataBuffer) > 0 {
					packet := make([]byte, len(dataBuffer))
					copy(packet, dataBuffer)
					si.ProcessIncoming(packet)
					dataBuffer = dataBuffer[:0]
				}
				inFrame = true
				escape = false
				continue
			}

			if inFrame {
				if b == KISSFesc {
					escape = true
				} else {
					if escape {
						if b == KISSTFend {
							b = KISSFend
						} else if b == KISSTFesc {
							b = KISSFesc
						}
						escape = false
					}
					dataBuffer = append(dataBuffer, b)
				}
			}
		}

		if si.uart.Buffered() == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// Send transmits data using KISS protocol with the default command 0x00.
func (si *SerialInterface) Send(data []byte, address string) error {
	return si.SendKISS(0x00, data)
}

// SendKISS sends a KISS-encoded packet over the serial UART.
func (si *SerialInterface) SendKISS(command byte, data []byte) error {
	si.Mutex.RLock()
	online := si.Online
	si.Mutex.RUnlock()

	if !online {
		return fmt.Errorf("interface offline")
	}

	frame := make([]byte, 0, len(data)*2+3)
	frame = append(frame, KISSFend)
	frame = append(frame, command)
	frame = append(frame, escapeKISS(data)...)
	frame = append(frame, KISSFend)

	_, err := si.uart.Write(frame)
	if err != nil {
		return err
	}

	si.Mutex.Lock()
	si.TxBytes += uint64(len(frame))
	si.lastTx = time.Now()
	si.Mutex.Unlock()

	return nil
}
