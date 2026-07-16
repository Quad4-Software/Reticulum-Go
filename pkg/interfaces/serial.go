// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build (linux || darwin || freebsd || openbsd || windows) && !js

package interfaces

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.bug.st/serial"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const (
	serialHWMTU            = 564
	serialDefaultSpeed     = 9600
	serialDefaultDataBits  = 8
	serialDefaultStopBits  = 1
	serialDefaultFrameIdle = 100 * time.Millisecond
	serialDefaultReconnect = 5 * time.Second
	serialReadChunk        = 4096
	serialDefaultIFACSize  = 8
)

// SerialPort is the byte stream behind SerialInterface. Tests inject pipes or
// PTYs. Production opens use go.bug.st/serial.
type SerialPort interface {
	io.ReadWriteCloser
}

// SerialOpenFunc opens a serial device from options.
type SerialOpenFunc func(opts SerialOptions) (SerialPort, error)

// SerialOptions configures SerialInterface beyond the Python baseline.
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
	Open              SerialOpenFunc
}

// SerialStats is atomic traffic and health counters for operators.
type SerialStats struct {
	FramesRX      atomic.Uint64
	FramesTX      atomic.Uint64
	BytesRX       atomic.Uint64
	BytesTX       atomic.Uint64
	FramingErrors atomic.Uint64
	Reconnects    atomic.Uint64
	OpenFailures  atomic.Uint64
}

// SerialInterface carries HDLC-framed Reticulum packets over a serial port.
// Beyond Python SerialInterface it adds chunked reads, configurable flow
// control, inter-byte frame idle drops, reconnect limits, IFAC, receive-only,
// injectable ports for tests, and live stats.
type SerialInterface struct {
	BaseInterface
	opts     SerialOptions
	port     SerialPort
	done     chan struct{}
	stopOnce sync.Once
	txMu     sync.Mutex
	txFrame  []byte
	readBuf  []byte
	Stats    SerialStats

	reconnectMu   sync.Mutex
	reconnecting  bool
	reconnectLeft int
}

// NewSerialInterface constructs a serial interface. Device is required unless
// opts.Open returns a port without consulting Device.
func NewSerialInterface(name string, enabled bool, opts SerialOptions) (*SerialInterface, error) {
	if opts.Open == nil && strings.TrimSpace(opts.Device) == "" {
		return nil, fmt.Errorf("no port specified for serial interface")
	}
	opts = normalizeSerialOptions(opts)
	si := &SerialInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeSerial, enabled),
		opts:          opts,
		done:          make(chan struct{}),
		txFrame:       make([]byte, 0, opts.MTU*2+4),
		readBuf:       make([]byte, serialReadChunk),
		reconnectLeft: NormalizeMaxReconnectTries(opts.MaxReconnectTries),
	}
	si.In = true
	si.Out = true
	si.MTU = opts.MTU
	si.Bitrate = opts.Bitrate
	if enabled {
		if err := si.openPort(); err != nil {
			return nil, err
		}
		si.startReadLoop()
	}
	return si, nil
}

func normalizeSerialOptions(opts SerialOptions) SerialOptions {
	if opts.Speed <= 0 {
		opts.Speed = serialDefaultSpeed
	}
	if opts.DataBits <= 0 {
		opts.DataBits = serialDefaultDataBits
	}
	if opts.StopBits <= 0 {
		opts.StopBits = serialDefaultStopBits
	}
	if opts.Parity == "" {
		opts.Parity = "N"
	}
	if opts.FrameIdle <= 0 {
		opts.FrameIdle = serialDefaultFrameIdle
	}
	if opts.ReconnectDelay <= 0 {
		opts.ReconnectDelay = serialDefaultReconnect
	}
	if opts.MTU <= 0 {
		opts.MTU = serialHWMTU
	}
	if opts.Bitrate <= 0 {
		opts.Bitrate = int64(opts.Speed)
	}
	if opts.Open == nil {
		opts.Open = openHardwareSerial
	}
	return opts
}

func openHardwareSerial(opts SerialOptions) (SerialPort, error) {
	mode := &serial.Mode{
		BaudRate: opts.Speed,
		DataBits: opts.DataBits,
		StopBits: serial.StopBits(opts.StopBits),
	}
	switch strings.ToLower(opts.Parity) {
	case "e", "even":
		mode.Parity = serial.EvenParity
	case "o", "odd":
		mode.Parity = serial.OddParity
	default:
		mode.Parity = serial.NoParity
	}
	p, err := serial.Open(opts.Device, mode)
	if err != nil {
		return nil, err
	}
	_ = p.SetReadTimeout(50 * time.Millisecond)
	return p, nil
}

func (si *SerialInterface) String() string {
	return fmt.Sprintf("SerialInterface[%s/%s]", si.Name, si.opts.Device)
}

func (si *SerialInterface) Start() error {
	si.Mutex.Lock()
	if si.Online {
		si.Mutex.Unlock()
		return nil
	}
	enabled := si.Enabled
	si.Mutex.Unlock()
	if !enabled {
		return fmt.Errorf("interface not enabled")
	}
	if err := si.openPort(); err != nil {
		return err
	}
	si.startReadLoop()
	return nil
}

func (si *SerialInterface) Stop() error {
	si.Mutex.Lock()
	si.Enabled = false
	si.Online = false
	si.Mutex.Unlock()
	si.stopOnce.Do(func() {
		close(si.done)
	})
	si.closePort()
	return nil
}

func (si *SerialInterface) openPort() error {
	port, err := si.opts.Open(si.opts)
	if err != nil {
		si.Stats.OpenFailures.Add(1)
		return err
	}
	si.Mutex.Lock()
	si.closePortLocked()
	si.port = port
	si.Online = true
	si.Mutex.Unlock()
	debug.Log(debug.DebugVerbose, "Serial port open", "name", si.Name, "device", si.opts.Device, "speed", si.opts.Speed)
	return nil
}

func (si *SerialInterface) closePort() {
	si.Mutex.Lock()
	defer si.Mutex.Unlock()
	si.closePortLocked()
}

func (si *SerialInterface) closePortLocked() {
	if si.port != nil {
		_ = si.port.Close()
		si.port = nil
	}
}

func (si *SerialInterface) ProcessOutgoing(data []byte) error {
	si.txMu.Lock()
	defer si.txMu.Unlock()

	si.Mutex.RLock()
	online := si.Online
	port := si.port
	si.Mutex.RUnlock()
	if !online || port == nil {
		return fmt.Errorf("serial interface offline")
	}
	frame := appendFrameHDLC(si.txFrame[:0], data)
	si.txFrame = frame
	written := 0
	for written < len(frame) {
		n, err := port.Write(frame[written:])
		written += n
		if err != nil {
			si.handleIOError(err)
			return err
		}
		if n == 0 {
			return fmt.Errorf("serial interface wrote 0 bytes")
		}
	}
	si.Stats.FramesTX.Add(1)
	si.Stats.BytesTX.Add(uint64(len(frame)))
	return nil
}

func (si *SerialInterface) Send(data []byte, _ string) error {
	if err := common.RejectReceiveOnly(si); err != nil {
		return err
	}
	masked, err := common.ApplyIFACOutbound(si, data)
	if err != nil {
		return err
	}
	if err := si.ProcessOutgoing(masked); err != nil {
		return err
	}
	si.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (si *SerialInterface) startReadLoop() {
	go si.readLoop()
}

func (si *SerialInterface) readLoop() {
	decoder := newHDLCStreamDecoder(si.MTU, func(payload []byte) {
		si.Stats.FramesRX.Add(1)
		si.Stats.BytesRX.Add(uint64(len(payload)))
		si.ProcessIncoming(payload)
	})
	idle := time.NewTimer(si.opts.FrameIdle)
	defer idle.Stop()
	drainTimer(idle)

	buf := si.readBuf
	for {
		si.Mutex.RLock()
		port := si.port
		done := si.done
		online := si.Online
		si.Mutex.RUnlock()
		if !online || port == nil {
			return
		}
		select {
		case <-done:
			return
		default:
		}

		n, err := port.Read(buf)
		if n > 0 {
			decoder.feed(buf[:n])
			drainTimer(idle)
			idle.Reset(si.opts.FrameIdle)
		}
		if err != nil {
			if isSerialTimeout(err) {
				select {
				case <-idle.C:
					if decoder.dropPartial() {
						si.Stats.FramingErrors.Add(1)
					}
				default:
				}
				drainTimer(idle)
				idle.Reset(si.opts.FrameIdle)
				continue
			}
			si.handleIOError(err)
			return
		}
		if n == 0 {
			select {
			case <-done:
				return
			case <-idle.C:
				if decoder.dropPartial() {
					si.Stats.FramingErrors.Add(1)
				}
				idle.Reset(si.opts.FrameIdle)
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
}

func drainTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}

func isSerialTimeout(err error) bool {
	if err == nil {
		return false
	}
	type timeout interface{ Timeout() bool }
	if t, ok := err.(timeout); ok && t.Timeout() {
		return true
	}
	return false
}

func (si *SerialInterface) handleIOError(err error) {
	debug.Log(debug.DebugError, "Serial interface I/O error", "name", si.Name, "error", err)
	si.Mutex.Lock()
	si.Online = false
	enabled := si.Enabled
	detached := si.Detached
	si.closePortLocked()
	si.Mutex.Unlock()
	if enabled && !detached {
		go si.reconnectLoop()
	}
}

func (si *SerialInterface) reconnectLoop() {
	si.reconnectMu.Lock()
	if si.reconnecting {
		si.reconnectMu.Unlock()
		return
	}
	si.reconnecting = true
	si.reconnectMu.Unlock()
	defer func() {
		si.reconnectMu.Lock()
		si.reconnecting = false
		si.reconnectMu.Unlock()
	}()

	tries := 0
	for {
		si.Mutex.RLock()
		enabled := si.Enabled
		detached := si.Detached
		done := si.done
		delay := si.opts.ReconnectDelay
		left := si.reconnectLeft
		si.Mutex.RUnlock()
		if !enabled || detached {
			return
		}
		if left == 0 {
			debug.Log(debug.DebugError, "Serial reconnect exhausted", "name", si.Name)
			_ = si.Stop()
			return
		}
		select {
		case <-done:
			return
		case <-time.After(delay):
		}
		tries++
		if left > 0 {
			si.Mutex.Lock()
			si.reconnectLeft--
			si.Mutex.Unlock()
		}
		debug.Log(debug.DebugVerbose, "Serial reconnect attempt", "name", si.Name, "try", tries)
		if err := si.openPort(); err != nil {
			debug.Log(debug.DebugError, "Serial reconnect failed", "name", si.Name, "error", err)
			continue
		}
		si.Stats.Reconnects.Add(1)
		si.Mutex.Lock()
		si.reconnectLeft = NormalizeMaxReconnectTries(si.opts.MaxReconnectTries)
		si.Mutex.Unlock()
		debug.Log(debug.DebugInfo, "Serial reconnected", "name", si.Name, "device", si.opts.Device)
		si.readLoop()
		return
	}
}

func (si *SerialInterface) GetConn() net.Conn { return nil }

func (si *SerialInterface) SendPathRequest([]byte) error { return nil }

func (si *SerialInterface) SendLinkPacket([]byte, []byte, time.Time) error { return nil }

func (si *SerialInterface) GetBandwidthAvailable() bool {
	si.Mutex.RLock()
	defer si.Mutex.RUnlock()
	return si.Online && si.port != nil
}

// Device returns the configured device path.
func (si *SerialInterface) Device() string { return si.opts.Device }

// Speed returns the configured baud rate.
func (si *SerialInterface) Speed() int { return si.opts.Speed }
