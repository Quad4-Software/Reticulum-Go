// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

type memSerialPair struct {
	aToB *bytePipe
	bToA *bytePipe
}

type bytePipe struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    bytes.Buffer
	closed bool
}

func newBytePipe() *bytePipe {
	p := &bytePipe{}
	p.cond = sync.NewCond(&p.mu)
	return p
}

func (p *bytePipe) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for p.buf.Len() == 0 && !p.closed {
		p.cond.Wait()
	}
	if p.buf.Len() == 0 && p.closed {
		return 0, io.EOF
	}
	return p.buf.Read(b)
}

func (p *bytePipe) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, io.ErrClosedPipe
	}
	n, err := p.buf.Write(b)
	p.cond.Signal()
	return n, err
}

func (p *bytePipe) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.cond.Broadcast()
	return nil
}

func newMemSerialPair() *memSerialPair {
	aToB := newBytePipe()
	bToA := newBytePipe()
	return &memSerialPair{aToB: aToB, bToA: bToA}
}

type duplexEnd struct {
	r *bytePipe
	w *bytePipe
}

func (d *duplexEnd) Read(b []byte) (int, error)  { return d.r.Read(b) }
func (d *duplexEnd) Write(b []byte) (int, error) { return d.w.Write(b) }
func (d *duplexEnd) Close() error {
	_ = d.r.Close()
	_ = d.w.Close()
	return nil
}

func (p *memSerialPair) ends() (portA, portB SerialPort) {
	return &duplexEnd{r: p.bToA, w: p.aToB}, &duplexEnd{r: p.aToB, w: p.bToA}
}

func TestSerialInterfaceRoundTrip(t *testing.T) {
	pair := newMemSerialPair()
	aPort, bPort := pair.ends()

	a, err := NewSerialInterface("a", true, SerialOptions{
		Device:    "mem-a",
		FrameIdle: 50 * time.Millisecond,
		Open:      func(SerialOptions) (SerialPort, error) { return aPort, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()

	b, err := NewSerialInterface("b", true, SerialOptions{
		Device:    "mem-b",
		FrameIdle: 50 * time.Millisecond,
		Open:      func(SerialOptions) (SerialPort, error) { return bPort, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	var got atomic.Pointer[[]byte]
	b.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		cp := append([]byte(nil), data...)
		got.Store(&cp)
	})

	payload := []byte{0x01, 0x7e, 0x7d, 0xff, 0x02}
	if err := a.Send(payload, ""); err != nil {
		t.Fatalf("Send: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if p := got.Load(); p != nil {
			if !bytes.Equal(*p, payload) {
				t.Fatalf("got %x want %x", *p, payload)
			}
			if a.Stats.FramesTX.Load() != 1 {
				t.Fatalf("tx frames=%d", a.Stats.FramesTX.Load())
			}
			if b.Stats.FramesRX.Load() != 1 {
				t.Fatalf("rx frames=%d", b.Stats.FramesRX.Load())
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timeout waiting for serial packet")
}

func TestSerialInterfaceReceiveOnly(t *testing.T) {
	pair := newMemSerialPair()
	aPort, _ := pair.ends()
	a, err := NewSerialInterface("ro", true, SerialOptions{
		Device: "mem",
		Open:   func(SerialOptions) (SerialPort, error) { return aPort, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	a.SetOutgoingAllowed(false)
	if err := a.Send([]byte("x"), ""); err == nil {
		t.Fatal("expected receive-only reject")
	}
}

func TestSerialFrameIdleDropsPartial(t *testing.T) {
	d := newHDLCStreamDecoder(564, func([]byte) {
		t.Fatal("should not complete partial frame")
	})
	d.feed([]byte{HDLCFlag, 0x01, 0x02})
	if !d.dropPartial() {
		t.Fatal("expected partial drop")
	}
	if d.inFrame || len(d.data) != 0 {
		t.Fatal("decoder not reset")
	}
}

func TestSerialRequiresDevice(t *testing.T) {
	_, err := NewSerialInterface("x", false, SerialOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSerialConcurrentSend(t *testing.T) {
	pair := newMemSerialPair()
	aPort, bPort := pair.ends()
	a, err := NewSerialInterface("a", true, SerialOptions{
		Device: "a",
		Open:   func(SerialOptions) (SerialPort, error) { return aPort, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	b, err := NewSerialInterface("b", true, SerialOptions{
		Device: "b",
		Open:   func(SerialOptions) (SerialPort, error) { return bPort, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	var n atomic.Int32
	b.SetPacketCallback(func([]byte, common.NetworkInterface) { n.Add(1) })

	const workers = 8
	const each = 20
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := range workers {
		go func(id int) {
			defer wg.Done()
			for i := range each {
				_ = a.Send([]byte{byte(id), byte(i)}, "")
			}
		}(w)
	}
	wg.Wait()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if int(n.Load()) >= workers*each {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("got %d want %d", n.Load(), workers*each)
}
