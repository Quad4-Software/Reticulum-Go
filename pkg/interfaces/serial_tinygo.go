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
	SERIAL_DEFAULT_BAUD = 115200
	SERIAL_MTU          = 1500
)

// SerialInterface implements a serial interface using TinyGo UART.
type SerialInterface struct {
	BaseInterface
	uart     *machine.UART
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
		BaseInterface: NewBaseInterface(name, common.IF_TYPE_SERIAL, enabled),
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

// getUART returns a TinyGo UART handle by name or index.
func getUART(name string) (*machine.UART, error) {
	switch name {
	case "UART0", "0":
		return machine.UART0, nil
	case "UART1", "1":
		return machine.UART1, nil
	case "UART2", "2":
		return machine.UART2, nil
	default:
		if name == "" {
			return machine.UART0, nil
		}
		return nil, fmt.Errorf("unknown UART: %s", name)
	}
}

// Start enables the serial interface and starts the read loop.
func (si *SerialInterface) Start() error {
	si.Mutex.Lock()
	defer si.Mutex.Unlock()

	if si.Online {
		return nil
	}

	err := si.uart.Configure(machine.UARTConfig{
		BaudRate: si.baud,
	})
	if err != nil {
		return fmt.Errorf("failed to configure UART: %w", err)
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
	buffer := make([]byte, si.MTU)
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

		if si.uart.Buffered() > 0 {
			n, err := si.uart.Read(buffer)
			if err != nil {
				debug.Log(debug.DEBUG_ERROR, "Serial read error", "name", si.Name, "error", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if n > 0 {
				for i := 0; i < n; i++ {
					b := buffer[i]

					if b == KISS_FEND {
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
						if b == KISS_FESC {
							escape = true
						} else {
							if escape {
								if b == KISS_TFEND {
									b = KISS_FEND
								} else if b == KISS_TFESC {
									b = KISS_FESC
								}
								escape = false
							}
							dataBuffer = append(dataBuffer, b)
						}
					}
				}
			}
		} else {
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
	frame = append(frame, KISS_FEND)
	frame = append(frame, command)
	frame = append(frame, escapeKISS(data)...)
	frame = append(frame, KISS_FEND)

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

