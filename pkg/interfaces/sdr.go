// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/sdr"
)

const (
	sdrDefaultIFACSize = 8
	sdrDefaultBitrate  = 1200
	sdrRXQueue         = 16
	sdrTXQueue         = 16
)

// SDROptions configures SDRInterface.
type SDROptions struct {
	Device     string
	Serial     string
	Address    string
	Frequency  int64
	SampleRate int
	Bandwidth  int
	RXGain     float64
	TXGain     float64
	Modem      string
	Bitrate    int64
	MTU        int
	// DeviceOverride injects a device (tests).
	DeviceOverride sdr.Device
}

// SDROptionsFromConfig maps InterfaceConfig into SDROptions.
func SDROptionsFromConfig(cfg *common.InterfaceConfig) SDROptions {
	if cfg == nil {
		return SDROptions{}
	}
	addr := cfg.Address
	if addr == "" && cfg.TargetHost != "" {
		if cfg.TargetPort != 0 {
			addr = net.JoinHostPort(cfg.TargetHost, fmt.Sprintf("%d", cfg.TargetPort))
		} else {
			addr = cfg.TargetHost
		}
	}
	return SDROptions{
		Device:     cfg.Device,
		Serial:     cfg.SerialNum,
		Address:    addr,
		Frequency:  cfg.FrequencyHz,
		SampleRate: cfg.SampleRate,
		Bandwidth:  cfg.Bandwidth,
		RXGain:     cfg.RXGain,
		TXGain:     cfg.TXGain,
		Modem:      cfg.Modem,
		Bitrate:    cfg.Bitrate,
		MTU:        cfg.MTU,
	}
}

// SDRInterface sends RNS packets over an SDR device using a Go burst modem.
//
// Lab and testing use. Prefer mock or sim for development. Live TX is not
// authorized by this package. Operators must follow local radio rules. The
// burst modem is not air-compatible with Modem73 or RNode.
type SDRInterface struct {
	BaseInterface

	opts   SDROptions
	dev    sdr.Device
	modem  *sdr.BurstModem
	ctx    context.Context
	cancel context.CancelFunc

	rxIQ chan []sdr.Complex64
	txIQ chan []sdr.Complex64

	wg sync.WaitGroup
}

// NewSDRInterface constructs an SDRInterface. Call Start to open the device.
// See SDRInterface for the lab and testing disclaimer.
func NewSDRInterface(name string, enabled bool, opts SDROptions) (*SDRInterface, error) {
	if opts.Device == "" {
		opts.Device = "mock"
	}
	if opts.Modem == "" {
		opts.Modem = "burst"
	}
	if opts.Modem != "burst" {
		return nil, fmt.Errorf("sdr modem %q not supported", opts.Modem)
	}
	if opts.Bitrate <= 0 {
		opts.Bitrate = sdrDefaultBitrate
	}
	si := &SDRInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeSDR, enabled),
		opts:          opts,
		modem:         sdr.NewBurstModem(),
		rxIQ:          make(chan []sdr.Complex64, sdrRXQueue),
		txIQ:          make(chan []sdr.Complex64, sdrTXQueue),
	}
	if opts.MTU > 0 {
		si.MTU = opts.MTU
	}
	si.Bitrate = opts.Bitrate
	return si, nil
}

func (si *SDRInterface) String() string {
	return fmt.Sprintf("SDRInterface[%s/%s]", si.Name, si.opts.Device)
}

func (si *SDRInterface) Start() error {
	si.Mutex.RLock()
	enabled := si.Enabled
	started := si.cancel != nil
	si.Mutex.RUnlock()
	if !enabled || started {
		return nil
	}
	si.Mutex.Lock()
	if si.cancel != nil {
		si.Mutex.Unlock()
		return nil
	}
	si.ctx, si.cancel = context.WithCancel(context.Background())
	si.Mutex.Unlock()

	dev := si.opts.DeviceOverride
	if dev == nil {
		var err error
		dev, err = sdr.Open(sdr.Config{
			Device:     si.opts.Device,
			Serial:     si.opts.Serial,
			Address:    si.opts.Address,
			Frequency:  si.opts.Frequency,
			SampleRate: si.opts.SampleRate,
			Bandwidth:  si.opts.Bandwidth,
			RXGain:     si.opts.RXGain,
			TXGain:     si.opts.TXGain,
		})
		if err != nil {
			si.cancel()
			si.cancel = nil
			return err
		}
	}
	if err := dev.Open(si.ctx); err != nil {
		si.cancel()
		si.cancel = nil
		return err
	}
	_ = dev.Tune(si.opts.Frequency)
	_ = dev.SetSampleRate(si.opts.SampleRate)
	_ = dev.SetBandwidth(si.opts.Bandwidth)
	_ = dev.SetRXGain(si.opts.RXGain)
	_ = dev.SetTXGain(si.opts.TXGain)

	si.dev = dev
	caps := dev.Caps()
	if caps.RX {
		if err := dev.StartRX(si.ctx, si.rxIQ); err != nil {
			_ = dev.Close()
			si.cancel()
			si.cancel = nil
			return err
		}
	}
	if caps.TX {
		if err := dev.StartTX(si.ctx, si.txIQ); err != nil {
			_ = dev.Close()
			si.cancel()
			si.cancel = nil
			return err
		}
	}

	si.Mutex.Lock()
	si.Online = true
	si.Detached = false
	si.Mutex.Unlock()

	si.wg.Go(func() {
		si.rxLoop()
	})
	return nil
}

func (si *SDRInterface) Stop() error {
	si.Detach()
	return nil
}

func (si *SDRInterface) Detach() {
	si.Mutex.Lock()
	if si.Detached {
		si.Mutex.Unlock()
		return
	}
	si.Detached = true
	si.Online = false
	cancel := si.cancel
	si.cancel = nil
	dev := si.dev
	si.Mutex.Unlock()
	if cancel != nil {
		cancel()
	}
	if dev != nil {
		_ = dev.Close()
	}
	si.wg.Wait()
}

func (si *SDRInterface) rxLoop() {
	for {
		select {
		case <-si.ctx.Done():
			return
		case block, ok := <-si.rxIQ:
			if !ok {
				return
			}
			if len(block) == 0 {
				continue
			}
			payload, ok := si.modem.Decode(block)
			if !ok {
				continue
			}
			si.ProcessIncoming(payload)
		}
	}
}

func (si *SDRInterface) ProcessOutgoing(data []byte) error {
	if si.ReceiveOnly {
		return errors.New("interface is receive-only")
	}
	if !si.IsOnline() {
		return errors.New("sdr interface offline")
	}
	samples, err := si.modem.Encode(data)
	if err != nil {
		return err
	}
	select {
	case si.txIQ <- samples:
		si.Mutex.Lock()
		si.TxBytes += uint64(len(data))
		si.TxPackets++
		si.Mutex.Unlock()
		return nil
	case <-si.ctx.Done():
		return context.Canceled
	default:
		debug.Log(debug.DebugError, "sdr tx queue full dropping packet", "name", si.Name)
		return errors.New("sdr tx queue full")
	}
}

func (si *SDRInterface) Send(data []byte, _ string) error {
	masked, err := common.ApplyIFACOutbound(si, data)
	if err != nil {
		return err
	}
	return si.ProcessOutgoing(masked)
}

func (si *SDRInterface) GetConn() net.Conn { return nil }

func (si *SDRInterface) SendPathRequest([]byte) error { return nil }

func (si *SDRInterface) SendLinkPacket([]byte, []byte, time.Time) error { return nil }

func (si *SDRInterface) GetBandwidthAvailable() bool {
	return si.IsOnline() && !si.ReceiveOnly
}
