// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const (
	modem73DefaultKISSPort    = 8001
	modem73DefaultControlPort = 8073
	modem73DefaultOverhead    = 15
	modem73DefaultBitrate     = 400
	modem73DefaultShortMTU    = 170
	modem73DefaultIFACSize    = 8
	modem73ControlReconnect   = 5 * time.Second
	modem73ControlDialTimeout = 5 * time.Second
	modem73ControlQueueDepth  = 32
	modem73MTUFloor           = 500

	modem73PktLinkRequest byte = 0x02
	modem73PktProof       byte = 0x03
	modem73CtxLRRTT       byte = 0xFE
	modem73CtxLRProof     byte = 0xFF
)

// Modem73DialFunc dials a TCP address. Tests inject fake networks.
type Modem73DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// Modem73Options configures Modem73Interface.
type Modem73Options struct {
	TargetHost        string
	TargetPort        int
	ControlHost       string
	ControlPort       int
	MTUOverhead       int
	Bitrate           int64
	AutoFragmentation bool
	ShortFrames       string
	ShortMTU          int
	HandshakeX2       bool
	ProofX2           bool
	AutoBitrate       bool
	CSMAOverhead      bool
	TimeoutMargin     float64
	MaxReconnectTries int
	Dial              Modem73DialFunc
	// PathTimeoutHook receives a suggested path-request timeout in seconds.
	PathTimeoutHook func(seconds int)
	// ControlDialTimeout bounds control and data dial attempts. Zero uses default.
	ControlDialTimeout time.Duration
}

type modem73CtrlCmd struct {
	msg  map[string]any
	done chan error
}

// Modem73Interface carries RNS packets to a modem73 instance over KISS TCP
// with a parallel JSON control channel for MTU bitrate and short-frame TX.
type Modem73Interface struct {
	BaseInterface

	opts Modem73Options

	ctx    context.Context
	cancel context.CancelFunc

	dataConn    net.Conn
	controlConn net.Conn
	dataMu      sync.Mutex
	ctrlMu      sync.Mutex

	ctrlQueue chan modem73CtrlCmd
	txFrame   []byte

	shortPolicy   string
	shortMTU      int
	shortOperMode atomic.Int64 // -1 means none
	fragTarget    atomic.Value // *bool or nil
	lastCfg       atomic.Value // map[string]any

	rssi atomic.Value // float64
	snr  atomic.Value // float64
	q    atomic.Value // float64 pointer-ish via float64 NaN sentinel

	ctrlReconnects atomic.Uint64
	configSyncs    atomic.Uint64
	shortHits      atomic.Uint64
	shortFallbacks atomic.Uint64
	txDuplicates   atomic.Uint64
	framingErrors  atomic.Uint64
	droppedTX      atomic.Uint64

	pathTimeoutSec atomic.Int64
	alwaysApplied  atomic.Bool

	wg sync.WaitGroup
}

// NewModem73Interface builds a Modem73Interface. Call Start to connect.
func NewModem73Interface(name string, enabled bool, opts Modem73Options) (*Modem73Interface, error) {
	opts = normalizeModem73Options(opts)
	if err := validateModem73Options(opts); err != nil {
		return nil, err
	}
	m := &Modem73Interface{
		BaseInterface: NewBaseInterface(name, common.IFTypeModem73, enabled),
		opts:          opts,
		ctrlQueue:     make(chan modem73CtrlCmd, modem73ControlQueueDepth),
		txFrame:       make([]byte, 0, DefaultMTU*2+4),
		shortPolicy:   opts.ShortFrames,
		shortMTU:      opts.ShortMTU,
	}
	m.MTU = modem73MTUFloor
	m.Bitrate = opts.Bitrate
	m.shortOperMode.Store(-1)
	m.rssi.Store(float64(0))
	m.snr.Store(float64(0))
	m.q.Store(float64(-1))
	return m, nil
}

func normalizeModem73Options(opts Modem73Options) Modem73Options {
	if opts.TargetHost == "" {
		opts.TargetHost = "127.0.0.1"
	}
	if opts.TargetPort == 0 {
		opts.TargetPort = modem73DefaultKISSPort
	}
	if opts.ControlHost == "" {
		opts.ControlHost = opts.TargetHost
	}
	if opts.ControlPort == 0 {
		opts.ControlPort = modem73DefaultControlPort
	}
	if opts.MTUOverhead <= 0 {
		opts.MTUOverhead = modem73DefaultOverhead
	}
	if opts.Bitrate <= 0 {
		opts.Bitrate = modem73DefaultBitrate
	}
	if opts.ShortMTU <= 0 {
		opts.ShortMTU = modem73DefaultShortMTU
	}
	if opts.ShortFrames == "" {
		opts.ShortFrames = "auto"
	}
	if opts.TimeoutMargin <= 0 {
		opts.TimeoutMargin = 0.35
	}
	if opts.TimeoutMargin < 0.05 {
		opts.TimeoutMargin = 0.05
	}
	if opts.TimeoutMargin > 1 {
		opts.TimeoutMargin = 1
	}
	if opts.Dial == nil {
		opts.Dial = (&net.Dialer{}).DialContext
	}
	if opts.ControlDialTimeout <= 0 {
		opts.ControlDialTimeout = modem73ControlDialTimeout
	}
	return opts
}

func validateModem73Options(opts Modem73Options) error {
	switch opts.ShortFrames {
	case "off", "auto", "always":
	default:
		return fmt.Errorf("modem73 short_frames must be off auto or always got %q", opts.ShortFrames)
	}
	if opts.TargetPort <= 0 || opts.ControlPort <= 0 {
		return errors.New("modem73 target_port and control_port must be positive")
	}
	return nil
}

func (m *Modem73Interface) String() string {
	return fmt.Sprintf("Modem73Interface[%s/%s:%d]", m.Name, m.opts.TargetHost, m.opts.TargetPort)
}

// PathTimeoutHint returns the suggested path-request timeout in seconds.
func (m *Modem73Interface) PathTimeoutHint() int {
	return int(m.pathTimeoutSec.Load())
}

// PhyStats returns last RSSI SNR and link quality from rx_frame events.
// Quality is -1 when unknown.
func (m *Modem73Interface) PhyStats() (rssi, snr, quality float64) {
	rssi, _ = m.rssi.Load().(float64)
	snr, _ = m.snr.Load().(float64)
	quality, _ = m.q.Load().(float64)
	return rssi, snr, quality
}

func (m *Modem73Interface) Start() error {
	m.Mutex.RLock()
	enabled := m.Enabled
	started := m.cancel != nil
	m.Mutex.RUnlock()
	if !enabled || started {
		return nil
	}
	m.Mutex.Lock()
	if m.cancel != nil {
		m.Mutex.Unlock()
		return nil
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.Online = false
	m.Detached = false
	m.Mutex.Unlock()

	m.probeInitialMTU()
	m.wg.Add(3)
	go func() {
		defer m.wg.Done()
		m.dataLoop()
	}()
	go func() {
		defer m.wg.Done()
		m.controlLoop()
	}()
	go func() {
		defer m.wg.Done()
		m.controlWriter()
	}()
	return nil
}

func (m *Modem73Interface) Stop() error {
	m.Detach()
	return nil
}

func (m *Modem73Interface) Detach() {
	m.Mutex.Lock()
	if m.Detached {
		m.Mutex.Unlock()
		return
	}
	m.Detached = true
	m.Online = false
	cancel := m.cancel
	m.cancel = nil
	m.Mutex.Unlock()
	if cancel != nil {
		cancel()
	}
	m.closeConns()
	m.wg.Wait()
}

func (m *Modem73Interface) closeConns() {
	m.dataMu.Lock()
	if m.dataConn != nil {
		_ = m.dataConn.Close()
		m.dataConn = nil
	}
	m.dataMu.Unlock()
	m.ctrlMu.Lock()
	if m.controlConn != nil {
		_ = m.controlConn.Close()
		m.controlConn = nil
	}
	m.ctrlMu.Unlock()
}

func (m *Modem73Interface) dial(ctx context.Context, host string, port int) (net.Conn, error) {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	return m.opts.Dial(ctx, "tcp", addr)
}

func (m *Modem73Interface) probeInitialMTU() {
	ctx, cancel := context.WithTimeout(context.Background(), m.opts.ControlDialTimeout)
	defer cancel()
	conn, err := m.dial(ctx, m.opts.ControlHost, m.opts.ControlPort)
	if err != nil {
		debug.Log(debug.DebugError, "modem73 initial control query failed", "err", err)
		m.applyMTU(modem73MTUFloor)
		return
	}
	defer conn.Close()
	if err := modem73WriteControl(conn, map[string]any{"cmd": "get_config"}); err != nil {
		m.applyMTU(modem73MTUFloor)
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(m.opts.ControlDialTimeout))
	msg, err := modem73ReadControl(conn)
	if err != nil {
		m.applyMTU(modem73MTUFloor)
		return
	}
	m.syncFromConfig(msg)
}

func (m *Modem73Interface) applyMTU(mtu int) {
	if mtu < modem73MTUFloor {
		mtu = modem73MTUFloor
	}
	m.Mutex.Lock()
	old := m.MTU
	m.MTU = mtu
	m.Mutex.Unlock()
	if old != mtu {
		debug.Log(debug.DebugInfo, "modem73 MTU updated", "name", m.Name, "old", old, "new", mtu)
	}
	m.applyPathTimeout()
}

func (m *Modem73Interface) applyBitrate(bps int64) {
	if bps <= 0 {
		return
	}
	m.Mutex.Lock()
	old := m.Bitrate
	m.Bitrate = bps
	m.Mutex.Unlock()
	if old != bps {
		debug.Log(debug.DebugInfo, "modem73 bitrate updated", "name", m.Name, "old", old, "new", bps)
	}
	m.applyPathTimeout()
}

func (m *Modem73Interface) applyPathTimeout() {
	m.Mutex.RLock()
	br := m.Bitrate
	mtu := m.MTU
	m.Mutex.RUnlock()
	sec := modem73PathRequestTimeoutSec(br, mtu)
	if sec <= 0 {
		return
	}
	m.pathTimeoutSec.Store(int64(sec))
	if m.opts.PathTimeoutHook != nil {
		m.opts.PathTimeoutHook(sec)
	}
}

func (m *Modem73Interface) dataLoop() {
	backoff := time.Second
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}
		ctx, cancel := context.WithTimeout(m.ctx, m.opts.ControlDialTimeout)
		conn, err := m.dial(ctx, m.opts.TargetHost, m.opts.TargetPort)
		cancel()
		if err != nil {
			select {
			case <-m.ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		m.dataMu.Lock()
		m.dataConn = conn
		m.dataMu.Unlock()
		m.Mutex.Lock()
		m.Online = true
		m.Mutex.Unlock()
		debug.Log(debug.DebugVerbose, "modem73 data port connected", "name", m.Name)

		m.readKISS(conn)

		m.Mutex.Lock()
		m.Online = false
		m.Mutex.Unlock()
		m.dataMu.Lock()
		if m.dataConn == conn {
			m.dataConn = nil
		}
		m.dataMu.Unlock()
		_ = conn.Close()
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (m *Modem73Interface) readKISS(conn net.Conn) {
	decoder := newKISSStreamDecoder(m.GetMTU(), func(payload []byte) {
		m.ProcessIncoming(append([]byte(nil), payload...))
	})
	buf := make([]byte, 4096)
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}
		n, err := conn.Read(buf)
		if n > 0 {
			mtu := m.GetMTU()
			if decoder.mtu != mtu && mtu > 0 {
				decoder.mtu = mtu
			}
			decoder.feed(buf[:n])
		}
		if err != nil {
			if !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
				m.framingErrors.Add(1)
			}
			return
		}
	}
}

func (m *Modem73Interface) controlLoop() {
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}
		ctx, cancel := context.WithTimeout(m.ctx, m.opts.ControlDialTimeout)
		conn, err := m.dial(ctx, m.opts.ControlHost, m.opts.ControlPort)
		cancel()
		if err != nil {
			m.ctrlReconnects.Add(1)
			select {
			case <-m.ctx.Done():
				return
			case <-time.After(modem73ControlReconnect):
			}
			continue
		}
		m.ctrlMu.Lock()
		m.controlConn = conn
		m.ctrlMu.Unlock()
		m.alwaysApplied.Store(false)
		debug.Log(debug.DebugVerbose, "modem73 control port connected", "name", m.Name)

		_ = m.enqueueControl(map[string]any{"cmd": "get_config"}, false)

		for {
			msg, err := modem73ReadControl(conn)
			if err != nil {
				break
			}
			m.handleControlMsg(msg)
		}

		m.ctrlMu.Lock()
		if m.controlConn == conn {
			m.controlConn = nil
		}
		m.ctrlMu.Unlock()
		_ = conn.Close()
		m.ctrlReconnects.Add(1)
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(modem73ControlReconnect):
		}
	}
}

func (m *Modem73Interface) controlWriter() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case cmd := <-m.ctrlQueue:
			m.ctrlMu.Lock()
			conn := m.controlConn
			m.ctrlMu.Unlock()
			var err error
			if conn == nil {
				err = errors.New("modem73 control not connected")
			} else {
				err = modem73WriteControl(conn, cmd.msg)
			}
			if cmd.done != nil {
				cmd.done <- err
			}
		}
	}
}

func (m *Modem73Interface) enqueueControl(msg map[string]any, wait bool) error {
	cmd := modem73CtrlCmd{msg: msg}
	if wait {
		cmd.done = make(chan error, 1)
	}
	select {
	case <-m.ctx.Done():
		return context.Canceled
	case m.ctrlQueue <- cmd:
	default:
		m.droppedTX.Add(1)
		return errors.New("modem73 control queue full")
	}
	if !wait {
		return nil
	}
	select {
	case <-m.ctx.Done():
		return context.Canceled
	case err := <-cmd.done:
		return err
	case <-time.After(m.opts.ControlDialTimeout):
		return errors.New("modem73 control write timeout")
	}
}

func (m *Modem73Interface) handleControlMsg(msg map[string]any) {
	if msg == nil {
		return
	}
	if ev, _ := msg["event"].(string); ev == "config_changed" {
		if cfg, ok := msg["config"].(map[string]any); ok {
			m.syncFromConfig(cfg)
		}
		return
	}
	if ev, _ := msg["event"].(string); ev == "rx_frame" {
		m.updateRXStats(msg)
		return
	}
	if _, ok := msg["payload_size"]; ok {
		m.syncFromConfig(msg)
	}
}

func (m *Modem73Interface) updateRXStats(msg map[string]any) {
	if level, ok := modem73CfgFloat(msg, "level_db"); ok {
		m.rssi.Store(mathRound1(level))
	}
	if snr, ok := modem73CfgFloat(msg, "snr"); ok {
		m.snr.Store(mathRound1(snr))
	}
	if ber, ok := modem73CfgFloat(msg, "ber_pct"); ok && ber >= 0 {
		q := 100.0 - ber
		if q < 0 {
			q = 0
		}
		if q > 100 {
			q = 100
		}
		m.q.Store(mathRound1(q))
	} else {
		m.q.Store(float64(-1))
	}
}

func mathRound1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func (m *Modem73Interface) syncFromConfig(cfg map[string]any) {
	if cfg == nil {
		return
	}
	m.lastCfg.Store(cfg)
	m.configSyncs.Add(1)
	if ps, ok := modem73CfgInt(cfg, "payload_size"); ok {
		mtu := modem73ComputeMTU(ps, m.opts.MTUOverhead, modem73MTUFloor)
		m.applyMTU(mtu)
		if m.opts.AutoFragmentation {
			want := modem73NeedsFragmentation(ps, m.opts.MTUOverhead, modem73MTUFloor)
			prev, _ := m.fragTarget.Load().(*bool)
			if prev == nil || *prev != want {
				if m.setFragmentation(want) {
					v := want
					m.fragTarget.Store(&v)
				}
			}
		}
	}
	if m.opts.AutoBitrate {
		if bps, ok := modem73TimeoutBitrate(cfg, m.opts.CSMAOverhead, m.opts.TimeoutMargin); ok {
			m.applyBitrate(int64(bps))
		}
	}
	if mode, ok := modem73ShortOperMode(cfg); ok {
		m.shortOperMode.Store(int64(mode))
	} else {
		m.shortOperMode.Store(-1)
	}
	if m.shortPolicy == "always" && !m.alwaysApplied.Load() {
		m.applyAlwaysShort(cfg)
	}
}

func (m *Modem73Interface) setFragmentation(enabled bool) bool {
	err := m.enqueueControl(map[string]any{
		"cmd":                   "set_config",
		"fragmentation_enabled": enabled,
	}, true)
	return err == nil
}

func (m *Modem73Interface) applyAlwaysShort(cfg map[string]any) {
	mt, ok := modem73CfgInt(cfg, "modem_type")
	if !ok {
		m.alwaysApplied.Store(true)
		return
	}
	var msg map[string]any
	switch mt {
	case modem73TypeRobust:
		rm, ok := modem73CfgInt(cfg, "robust_mode")
		if ok && rm < modem73RobustShortOffset {
			msg = map[string]any{"cmd": "set_config", "robust_mode": rm + modem73RobustShortOffset}
		}
	case modem73TypeOFDM:
		if !modem73CfgBool(cfg, "short_frame", false) {
			msg = map[string]any{"cmd": "set_config", "short_frame": true}
		}
	}
	if msg == nil {
		m.alwaysApplied.Store(true)
		return
	}
	if err := m.enqueueControl(msg, true); err == nil {
		m.alwaysApplied.Store(true)
	}
}

func (m *Modem73Interface) ProcessOutgoing(data []byte) error {
	if m.ReceiveOnly {
		return errors.New("interface is receive-only")
	}
	copies := 1
	if m.GetIFAC() == nil {
		if m.opts.HandshakeX2 && modem73IsHandshake(data) {
			copies = 2
		} else if m.opts.ProofX2 && modem73IsProof(data) {
			copies = 2
		}
	}
	var firstErr error
	for i := 0; i < copies; i++ {
		if i > 0 {
			m.txDuplicates.Add(1)
		}
		if err := m.sendOne(data); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *Modem73Interface) sendOne(data []byte) error {
	mode := m.shortOperMode.Load()
	if m.shortPolicy == "auto" && mode >= 0 && len(data) <= m.shortMTU && m.IsOnline() {
		if m.sendShortFrame(data, int(mode)) {
			m.shortHits.Add(1)
			return nil
		}
		m.shortFallbacks.Add(1)
	}
	return m.sendKISS(data)
}

func (m *Modem73Interface) sendShortFrame(data []byte, operMode int) bool {
	msg := map[string]any{
		"cmd":       "tx",
		"data":      base64.StdEncoding.EncodeToString(data),
		"oper_mode": operMode,
	}
	if err := m.enqueueControl(msg, true); err != nil {
		return false
	}
	m.Mutex.Lock()
	m.TxBytes += uint64(len(data))
	m.TxPackets++
	m.Mutex.Unlock()
	return true
}

func (m *Modem73Interface) sendKISS(data []byte) error {
	m.dataMu.Lock()
	conn := m.dataConn
	m.dataMu.Unlock()
	if conn == nil {
		return errors.New("modem73 data port not connected")
	}
	m.Mutex.Lock()
	frame := appendFrameKISS(m.txFrame[:0], data)
	m.txFrame = frame
	out := append([]byte(nil), frame...)
	m.Mutex.Unlock()
	_, err := conn.Write(out)
	if err != nil {
		return err
	}
	m.Mutex.Lock()
	m.TxBytes += uint64(len(data))
	m.TxPackets++
	m.Mutex.Unlock()
	return nil
}

func (m *Modem73Interface) Send(data []byte, _ string) error {
	masked, err := common.ApplyIFACOutbound(m, data)
	if err != nil {
		return err
	}
	return m.ProcessOutgoing(masked)
}

func (m *Modem73Interface) GetConn() net.Conn {
	m.dataMu.Lock()
	defer m.dataMu.Unlock()
	return m.dataConn
}

func (m *Modem73Interface) SendPathRequest([]byte) error { return nil }

func (m *Modem73Interface) SendLinkPacket([]byte, []byte, time.Time) error { return nil }

func (m *Modem73Interface) GetBandwidthAvailable() bool {
	return m.IsOnline() && !m.ReceiveOnly
}

func modem73IsHandshake(data []byte) bool {
	if len(data) < 19 {
		return false
	}
	flags := data[0]
	if (flags & 0x03) == modem73PktLinkRequest {
		return true
	}
	ctxOffset := 18
	if (flags>>6)&0x01 != 0 {
		ctxOffset = 34
	}
	if len(data) <= ctxOffset {
		return false
	}
	ctx := data[ctxOffset]
	return ctx == modem73CtxLRRTT || ctx == modem73CtxLRProof
}

func modem73IsProof(data []byte) bool {
	if len(data) < 19 {
		return false
	}
	return (data[0] & 0x03) == modem73PktProof
}
