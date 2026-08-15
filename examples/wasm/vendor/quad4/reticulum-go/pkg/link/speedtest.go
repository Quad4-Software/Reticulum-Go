// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

// DefaultSpeedtestDataCap is how many plaintext bytes to transfer in a
// loopback speedtest when the caller does not set DataCap.
const DefaultSpeedtestDataCap int64 = 2 << 20 // 2 MiB

// DefaultSpeedtestMinBytesPerSec is a soft loopback liveness floor. Real
// hardware and CI runners vary. This only catches catastrophic regressions.
const DefaultSpeedtestMinBytesPerSec = 1e6 // 1 MB/s

// SpeedtestOptions configures a link packet blast (RNS Speedtest-style).
type SpeedtestOptions struct {
	// DataCap is plaintext bytes to deliver (default DefaultSpeedtestDataCap).
	DataCap int64
	// MinBytesPerSec fails the run when sustained RX rate is below this
	// (0 disables the floor, default DefaultSpeedtestMinBytesPerSec when
	// EnforceFloor is true).
	MinBytesPerSec float64
	// EnforceFloor applies MinBytesPerSec (default true for smoke/CLI).
	EnforceFloor bool
	// Timeout bounds establish + transfer (default 30s).
	Timeout time.Duration
	// SendPace sleeps after each outbound packet. Zero means no pacing
	// (fine for in-process pipes, UDP needs a small delay to avoid drops).
	SendPace time.Duration
	// Ready, if non-nil, receives one value after the packet callback is installed
	// so a peer blast can start without racing the first packets.
	Ready chan<- struct{}
}

// SpeedtestResult is the outcome of a loopback or paired-link speedtest.
type SpeedtestResult struct {
	BytesSent   int64
	BytesRecv   int64
	Duration    time.Duration
	BytesPerSec float64
	MDU         int
	// ConfirmedRecv is the peer-reported RX byte count from the SPEEDOK ack
	// (client side only). Zero when no ack was received.
	ConfirmedRecv int64
}

// GetMDU returns the current link maximum data unit for plaintext packets.
func (l *Link) GetMDU() int {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.mdu
}

const speedAckMagic = "SPEEDOK"

// BlastOnLink sends DataCap plaintext bytes as MDU-sized packets (receipts off)
// and measures sender-side throughput. Used by cross-host clients.
func BlastOnLink(initiator *Link, opt SpeedtestOptions) (SpeedtestResult, error) {
	if initiator == nil {
		return SpeedtestResult{}, errors.New("speedtest: nil link")
	}
	opt = normalizeSpeedtestOptions(opt)
	mdu := initiator.GetMDU()
	if mdu <= 0 {
		return SpeedtestResult{}, fmt.Errorf("speedtest: invalid mdu %d", mdu)
	}
	payload := make([]byte, mdu)
	if _, err := rand.Read(payload); err != nil {
		return SpeedtestResult{}, fmt.Errorf("speedtest: rand: %w", err)
	}

	deadline := time.Now().Add(opt.Timeout)
	start := time.Now()
	var sent int64
	for sent < opt.DataCap {
		if time.Now().After(deadline) {
			return SpeedtestResult{
				BytesSent: sent,
				Duration:  time.Since(start),
				MDU:       mdu,
			}, fmt.Errorf("speedtest: send timeout after %d/%d bytes", sent, opt.DataCap)
		}
		if initiator.GetStatus() != StatusActive {
			return SpeedtestResult{}, errors.New("speedtest: initiator link not active")
		}
		chunk := payload
		remain := opt.DataCap - sent
		if remain < int64(len(chunk)) {
			chunk = payload[:remain]
		}
		if err := initiator.SendPacket(chunk); err != nil {
			return SpeedtestResult{}, fmt.Errorf("speedtest: send: %w", err)
		}
		sent += int64(len(chunk))
		if opt.SendPace > 0 {
			time.Sleep(opt.SendPace)
		}
	}
	elapsed := time.Since(start)
	bps := 0.0
	if elapsed > 0 {
		bps = float64(sent) / elapsed.Seconds()
	}
	res := SpeedtestResult{
		BytesSent:   sent,
		Duration:    elapsed,
		BytesPerSec: bps,
		MDU:         mdu,
	}
	if opt.EnforceFloor && bps < opt.MinBytesPerSec {
		return res, fmt.Errorf("speedtest: %.0f B/s below floor %.0f B/s", bps, opt.MinBytesPerSec)
	}
	return res, nil
}

// ReceiveOnLink counts inbound plaintext until DataCap bytes arrive (or timeout)
// and measures receiver-side throughput. Used by cross-host servers.
func ReceiveOnLink(responder *Link, opt SpeedtestOptions) (SpeedtestResult, error) {
	if responder == nil {
		return SpeedtestResult{}, errors.New("speedtest: nil link")
	}
	opt = normalizeSpeedtestOptions(opt)
	mdu := responder.GetMDU()

	var recv atomic.Int64
	done := make(chan struct{}, 1)
	responder.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		if len(data) >= len(speedAckMagic) && string(data[:len(speedAckMagic)]) == speedAckMagic {
			return
		}
		n := recv.Add(int64(len(data)))
		if n >= opt.DataCap {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	})
	if opt.Ready != nil {
		select {
		case opt.Ready <- struct{}{}:
		default:
		}
	}

	start := time.Now()
	deadline := time.Now().Add(opt.Timeout)
	select {
	case <-done:
	case <-time.After(time.Until(deadline)):
		got := recv.Load()
		return SpeedtestResult{
			BytesRecv: got,
			Duration:  time.Since(start),
			MDU:       mdu,
		}, fmt.Errorf("speedtest: recv timeout after %d/%d bytes", got, opt.DataCap)
	}

	elapsed := time.Since(start)
	got := recv.Load()
	bps := 0.0
	if elapsed > 0 {
		bps = float64(got) / elapsed.Seconds()
	}
	res := SpeedtestResult{
		BytesRecv:   got,
		Duration:    elapsed,
		BytesPerSec: bps,
		MDU:         mdu,
	}
	if opt.EnforceFloor && bps < opt.MinBytesPerSec {
		return res, fmt.Errorf("speedtest: %.0f B/s below floor %.0f B/s", bps, opt.MinBytesPerSec)
	}
	return res, nil
}

// SendSpeedAck notifies the peer how many plaintext bytes were received.
func SendSpeedAck(l *Link, recvBytes int64) error {
	if l == nil {
		return errors.New("speedtest: nil link")
	}
	buf := make([]byte, len(speedAckMagic)+8)
	copy(buf, speedAckMagic)
	binary.BigEndian.PutUint64(buf[len(speedAckMagic):], uint64(recvBytes)) // #nosec G115
	return l.SendPacket(buf)
}

// WaitSpeedAck waits for a SPEEDOK ack and returns the peer's reported RX count.
func WaitSpeedAck(l *Link, timeout time.Duration) (int64, error) {
	if l == nil {
		return 0, errors.New("speedtest: nil link")
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ch := make(chan int64, 1)
	l.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		if len(data) < len(speedAckMagic)+8 {
			return
		}
		if string(data[:len(speedAckMagic)]) != speedAckMagic {
			return
		}
		n := int64(binary.BigEndian.Uint64(data[len(speedAckMagic):])) // #nosec G115
		select {
		case ch <- n:
		default:
		}
	})
	select {
	case n := <-ch:
		return n, nil
	case <-time.After(timeout):
		return 0, errors.New("speedtest: ack timeout")
	}
}

// RunSpeedtest blasts MDU-sized link data packets from initiator to responder
// until DataCap plaintext bytes are received (or timeout). Receipts are off,
// matching Python RNS Examples/Speedtest.py. Both ends must be local links.
func RunSpeedtest(initiator, responder *Link, opt SpeedtestOptions) (SpeedtestResult, error) {
	if initiator == nil || responder == nil {
		return SpeedtestResult{}, errors.New("speedtest: nil link")
	}
	opt = normalizeSpeedtestOptions(opt)

	ready := make(chan struct{}, 1)
	recvOpt := opt
	recvOpt.Ready = ready
	recvOpt.EnforceFloor = false

	recvDone := make(chan SpeedtestResult, 1)
	recvErr := make(chan error, 1)
	go func() {
		res, err := ReceiveOnLink(responder, recvOpt)
		if err != nil {
			recvErr <- err
			return
		}
		recvDone <- res
	}()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		return SpeedtestResult{}, errors.New("speedtest: receiver not ready")
	}

	blastOpt := opt
	blastOpt.EnforceFloor = false
	sentRes, err := BlastOnLink(initiator, blastOpt)
	if err != nil {
		return sentRes, err
	}

	select {
	case res := <-recvDone:
		res.BytesSent = sentRes.BytesSent
		res.MDU = sentRes.MDU
		if opt.EnforceFloor && res.BytesPerSec < opt.MinBytesPerSec {
			return res, fmt.Errorf("speedtest: %.0f B/s below floor %.0f B/s", res.BytesPerSec, opt.MinBytesPerSec)
		}
		return res, nil
	case err := <-recvErr:
		sentRes.BytesRecv = 0
		return sentRes, err
	case <-time.After(opt.Timeout):
		sentRes.BytesRecv = 0
		return sentRes, errors.New("speedtest: recv wait timeout")
	}
}

// RunLoopbackSpeedtest establishes an in-process pipe link pair and runs
// RunSpeedtest. Suitable for CLI self-checks and CI liveness floors.
func RunLoopbackSpeedtest(opt SpeedtestOptions) (SpeedtestResult, error) {
	opt = normalizeSpeedtestOptions(opt)

	cfgA := &common.ReticulumConfig{}
	trA := transport.NewTransport(cfgA)
	cfgB := &common.ReticulumConfig{}
	trB := transport.NewTransport(cfgB)

	pa := newSpeedPipe("speedpipeA")
	pb := newSpeedPipe("speedpipeB")
	pa.peer = pb
	pb.peer = pa
	pa.tr = trA
	pb.tr = trB

	if err := trA.RegisterInterface(pa.Name, pa); err != nil {
		_ = trA.Close()
		_ = trB.Close()
		return SpeedtestResult{}, err
	}
	if err := trB.RegisterInterface(pb.Name, pb); err != nil {
		_ = trA.Close()
		_ = trB.Close()
		return SpeedtestResult{}, err
	}

	cleanup := func() {
		_ = trA.Close()
		_ = trB.Close()
	}
	defer cleanup()

	idA, err := identity.New()
	if err != nil {
		return SpeedtestResult{}, err
	}
	destA, err := destination.New(idA, destination.In, destination.Single, "speedtest", trA, "loopback")
	if err != nil {
		return SpeedtestResult{}, err
	}
	destA.AcceptsLinks(true)

	var responder *Link
	estA := make(chan struct{}, 1)
	destA.SetLinkEstablishedCallback(func(v any) {
		if lnk, ok := v.(*Link); ok {
			responder = lnk
			select {
			case estA <- struct{}{}:
			default:
			}
		}
	})

	if err := destA.Announce(false, nil, nil); err != nil {
		return SpeedtestResult{}, err
	}
	time.Sleep(50 * time.Millisecond)

	estB := make(chan struct{}, 1)
	initiator := NewLink(destA, trB, pb, func(*Link) {
		select {
		case estB <- struct{}{}:
		default:
		}
	}, nil)
	if err := initiator.Establish(); err != nil {
		return SpeedtestResult{}, err
	}

	establishDeadline := time.After(min(opt.Timeout, 15*time.Second))
	select {
	case <-estB:
	case <-establishDeadline:
		return SpeedtestResult{}, errors.New("speedtest: initiator establish timeout")
	}
	select {
	case <-estA:
	case <-time.After(min(opt.Timeout, 15*time.Second)):
		return SpeedtestResult{}, errors.New("speedtest: responder establish timeout")
	}
	if responder == nil {
		return SpeedtestResult{}, errors.New("speedtest: nil responder")
	}

	initiator.Start()
	responder.Start()
	return RunSpeedtest(initiator, responder, opt)
}

func normalizeSpeedtestOptions(opt SpeedtestOptions) SpeedtestOptions {
	if opt.DataCap <= 0 {
		opt.DataCap = DefaultSpeedtestDataCap
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 30 * time.Second
	}
	if opt.EnforceFloor && opt.MinBytesPerSec <= 0 {
		opt.MinBytesPerSec = DefaultSpeedtestMinBytesPerSec
	}
	return opt
}

// speedPipe is a minimal bidirectional in-process interface for loopback tests.
type speedPipe struct {
	common.BaseInterface
	peer   *speedPipe
	tr     *transport.Transport
	online bool
}

func newSpeedPipe(name string) *speedPipe {
	return &speedPipe{
		BaseInterface: common.BaseInterface{
			Name:    name,
			Type:    common.IFTypeUDP,
			Enabled: true,
			Online:  true,
			Bitrate: 1_000_000_000,
			MTU:     common.DefaultMTU,
		},
		online: true,
	}
}

func (p *speedPipe) Send(data []byte, _ string) error {
	if !p.online || p.peer == nil || !p.peer.online || p.peer.tr == nil {
		return errors.New("speedtest: pipe peer not connected")
	}
	raw := append([]byte(nil), data...)
	p.peer.tr.HandlePacket(raw, p.peer)
	return nil
}

func (p *speedPipe) IsEnabled() bool { return p.Enabled }
func (p *speedPipe) IsOnline() bool  { return p.online }
func (p *speedPipe) GetName() string { return p.Name }
func (p *speedPipe) Start() error    { return nil }
func (p *speedPipe) Stop() error     { p.online = false; return nil }
func (p *speedPipe) Detach()         { p.online = false }
