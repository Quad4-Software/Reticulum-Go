// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sdr

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

func init() {
	RegisterOpener("rtltcp", func(cfg Config) (Device, error) {
		return NewRTLTCP(cfg), nil
	})
}

// RTLTCP talks to an rtl_tcp server (RX only).
type RTLTCP struct {
	cfg  Config
	mu   sync.Mutex
	conn net.Conn
	open bool
	freq int64
	rate int
	gain float64
}

// NewRTLTCP builds an rtl_tcp client device.
func NewRTLTCP(cfg Config) *RTLTCP {
	cfg = normalizeConfig(cfg)
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:1234"
	}
	return &RTLTCP{cfg: cfg, freq: cfg.Frequency, rate: cfg.SampleRate, gain: cfg.RXGain}
}

func (d *RTLTCP) Caps() Caps {
	return Caps{
		RX: true, TX: false,
		MinFreqHz: 24_000_000, MaxFreqHz: 1_700_000_000,
		MinRate: 250000, MaxRate: 3200000,
		DeviceType: "rtltcp",
	}
}

func (d *RTLTCP) Open(ctx context.Context) error {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", d.cfg.Address)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.conn = conn
	d.open = true
	d.mu.Unlock()
	_ = d.Tune(d.freq)
	_ = d.SetSampleRate(d.rate)
	_ = d.SetRXGain(d.gain)
	return nil
}

func (d *RTLTCP) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.open = false
	if d.conn != nil {
		err := d.conn.Close()
		d.conn = nil
		return err
	}
	return nil
}

func (d *RTLTCP) cmd(typ byte, param uint32) error {
	d.mu.Lock()
	conn := d.conn
	d.mu.Unlock()
	if conn == nil {
		return errNotOpen
	}
	var buf [5]byte
	buf[0] = typ
	binary.BigEndian.PutUint32(buf[1:], param)
	_, err := conn.Write(buf[:])
	return err
}

func (d *RTLTCP) Tune(freqHz int64) error {
	d.freq = ClampFrequency(freqHz)
	return d.cmd(0x01, uint32(d.freq)) // #nosec G115 -- freq clamped to [0, MaxUint32]
}

func (d *RTLTCP) SetSampleRate(rate int) error {
	d.rate = ClampSampleRate(rate)
	return d.cmd(0x02, uint32(d.rate)) // #nosec G115 -- rate clamped to [250k, 20M]
}

func (d *RTLTCP) SetBandwidth(int) error { return nil }

func (d *RTLTCP) SetRXGain(db float64) error {
	d.gain = ClampGain(db)
	// rtl_tcp gain is tenths of dB.
	return d.cmd(0x04, uint32(d.gain*10)) // #nosec G115 -- gain clamped to [0, 60]
}

func (d *RTLTCP) SetTXGain(float64) error { return errRXOnly }

func (d *RTLTCP) StartRX(ctx context.Context, out chan<- []Complex64) error {
	d.mu.Lock()
	conn := d.conn
	open := d.open
	d.mu.Unlock()
	if !open || conn == nil {
		return errNotOpen
	}
	go func() {
		buf := make([]byte, 16384)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			n, err := conn.Read(buf)
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
					return
				}
				return
			}
			if n < 2 {
				continue
			}
			n -= n % 2
			samples := make([]Complex64, n/2)
			for i := 0; i+1 < n; i += 2 {
				samples[i/2] = Complex64{
					I: (float32(buf[i]) - 127.5) / 127.5,
					Q: (float32(buf[i+1]) - 127.5) / 127.5,
				}
			}
			select {
			case out <- samples:
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	return nil
}

func (d *RTLTCP) StartTX(context.Context, <-chan []Complex64) error {
	return fmt.Errorf("rtltcp: %w", errRXOnly)
}
