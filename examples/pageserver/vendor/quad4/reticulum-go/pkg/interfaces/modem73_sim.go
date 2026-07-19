// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"context"
	"encoding/base64"
	"errors"
	"maps"
	"math"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Modem73SimConfig configures the math-backed modem73 TNC simulator.
type Modem73SimConfig struct {
	Callsign       string
	ModemType      int
	RobustMode     int
	MFSKMode       int
	Modulation     string
	CodeRate       string
	FrameSize      int
	CenterFreq     int
	CSMAEnabled    bool
	CarrierThresh  float64
	PPersistence   int
	SlotTimeMs     int
	CSMAQuietMs    int
	CSMACW         int
	CSMABurst      int
	Fragmentation  bool
	SNRdB          float64 // channel SNR for BER model
	Seed           int64
	InstantAirtime bool // skip real-time airtime sleeps (tests)
}

// Modem73Channel is a shared RF medium between simulators.
type Modem73Channel struct {
	mu        sync.Mutex
	snrDB     float64
	rng       *rand.Rand
	subs      []chan modem73AirFrame
	busyUntil time.Time
}

type modem73AirFrame struct {
	payload []byte
	snrDB   float64
	from    *Modem73Simulator
}

// NewModem73Channel builds a shared channel with the given SNR.
func NewModem73Channel(snrDB float64, seed int64) *Modem73Channel {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &Modem73Channel{
		snrDB: snrDB,
		rng:   rand.New(rand.NewSource(seed)), // #nosec G404 -- modem BER sim seed, not crypto
	}
}

func (ch *Modem73Channel) subscribe() chan modem73AirFrame {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	c := make(chan modem73AirFrame, 16)
	ch.subs = append(ch.subs, c)
	return c
}

func (ch *Modem73Channel) publish(fr modem73AirFrame, airtime time.Duration) {
	ch.mu.Lock()
	ch.busyUntil = time.Now().Add(airtime)
	subs := append([]chan modem73AirFrame(nil), ch.subs...)
	ch.mu.Unlock()
	for _, s := range subs {
		if fr.from != nil {
			// deliver to all including sender loop filter
		}
		select {
		case s <- fr:
		default:
		}
	}
}

func (ch *Modem73Channel) isBusy() bool {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return time.Now().Before(ch.busyUntil)
}

func (ch *Modem73Channel) snr() float64 {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.snrDB
}

func (ch *Modem73Channel) SetSNR(snrDB float64) {
	ch.mu.Lock()
	ch.snrDB = snrDB
	ch.mu.Unlock()
}

func (ch *Modem73Channel) randFloat() float64 {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.rng.Float64()
}

// Modem73Simulator is a control+KISS TNC that models modem73 PHY math without radio hardware.
type Modem73Simulator struct {
	cfg Modem73SimConfig
	ch  *Modem73Channel

	kissLn net.Listener
	ctrlLn net.Listener

	mu     sync.Mutex
	cfgMap map[string]any

	rxFrames  atomic.Uint64
	txFrames  atomic.Uint64
	rxErrors  atomic.Uint64
	syncCount atomic.Uint64
	crcErrors atomic.Uint64
	lastSNR   atomic.Uint64 // float bits
	lastBER   atomic.Uint64
	pttOn     atomic.Bool
	state     atomic.Value // string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	kissConns map[net.Conn]struct{}
	ctrlConns map[net.Conn]struct{}
	connMu    sync.Mutex

	airSub chan modem73AirFrame
}

// NewModem73Simulator builds a simulator. Call Listen then Serve.
func NewModem73Simulator(cfg Modem73SimConfig, ch *Modem73Channel) *Modem73Simulator {
	if cfg.Callsign == "" {
		cfg.Callsign = "SIM73"
	}
	if cfg.Modulation == "" {
		cfg.Modulation = "QPSK"
	}
	if cfg.CodeRate == "" {
		cfg.CodeRate = "1/2"
	}
	if cfg.FrameSize < 0 || cfg.FrameSize > 2 {
		cfg.FrameSize = 1
	}
	if cfg.CenterFreq == 0 {
		cfg.CenterFreq = 1500
	}
	if cfg.PPersistence == 0 {
		cfg.PPersistence = 64
	}
	if cfg.SlotTimeMs == 0 {
		cfg.SlotTimeMs = 500
	}
	if cfg.CSMACW == 0 {
		cfg.CSMACW = 8
	}
	if cfg.CSMABurst == 0 {
		cfg.CSMABurst = 1
	}
	if ch == nil {
		ch = NewModem73Channel(cfg.SNRdB, cfg.Seed)
	}
	s := &Modem73Simulator{
		cfg:       cfg,
		ch:        ch,
		kissConns: map[net.Conn]struct{}{},
		ctrlConns: map[net.Conn]struct{}{},
		airSub:    ch.subscribe(),
	}
	s.state.Store("idle")
	s.rebuildConfig()
	return s
}

func (s *Modem73Simulator) rebuildConfig() {
	phy := Modem73ComputePhy(s.cfg.ModemType, s.cfg.RobustMode, s.cfg.MFSKMode, s.cfg.FrameSize, s.cfg.Modulation, s.cfg.CodeRate)
	s.cfgMap = map[string]any{
		"ok":                    true,
		"callsign":              s.cfg.Callsign,
		"modem_type":            float64(s.cfg.ModemType),
		"robust_mode":           float64(s.cfg.RobustMode),
		"mfsk_mode":             float64(s.cfg.MFSKMode),
		"modulation":            s.cfg.Modulation,
		"code_rate":             s.cfg.CodeRate,
		"short_frame":           s.cfg.FrameSize == 0,
		"frame_size":            float64(s.cfg.FrameSize),
		"postamble":             false,
		"center_freq":           float64(s.cfg.CenterFreq),
		"payload_size":          float64(phy.PayloadSize),
		"csma_enabled":          s.cfg.CSMAEnabled,
		"carrier_threshold_db":  s.cfg.CarrierThresh,
		"p_persistence":         float64(s.cfg.PPersistence),
		"slot_time_ms":          float64(s.cfg.SlotTimeMs),
		"csma_quiet_ms":         float64(s.cfg.CSMAQuietMs),
		"csma_cw":               float64(s.cfg.CSMACW),
		"csma_burst":            float64(s.cfg.CSMABurst),
		"tx_blanking_enabled":   true,
		"fragmentation_enabled": s.cfg.Fragmentation,
	}
}

// Listen binds KISS and control TCP listeners on ephemeral ports.
func (s *Modem73Simulator) Listen() (kissAddr, ctrlAddr string, err error) {
	s.kissLn, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", err
	}
	s.ctrlLn, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = s.kissLn.Close()
		return "", "", err
	}
	return s.kissLn.Addr().String(), s.ctrlLn.Addr().String(), nil
}

// KISSPort returns the bound KISS TCP port.
func (s *Modem73Simulator) KISSPort() int {
	if s.kissLn == nil {
		return 0
	}
	return s.kissLn.Addr().(*net.TCPAddr).Port
}

// ControlPort returns the bound control TCP port.
func (s *Modem73Simulator) ControlPort() int {
	if s.ctrlLn == nil {
		return 0
	}
	return s.ctrlLn.Addr().(*net.TCPAddr).Port
}

// Serve starts accept loops and the air receive path.
func (s *Modem73Simulator) Serve(ctx context.Context) {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(3)
	go func() {
		defer s.wg.Done()
		s.acceptKISS()
	}()
	go func() {
		defer s.wg.Done()
		s.acceptControl()
	}()
	go func() {
		defer s.wg.Done()
		s.airRX()
	}()
}

// Close stops the simulator.
func (s *Modem73Simulator) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.kissLn != nil {
		_ = s.kissLn.Close()
	}
	if s.ctrlLn != nil {
		_ = s.ctrlLn.Close()
	}
	s.connMu.Lock()
	for c := range s.kissConns {
		_ = c.Close()
	}
	for c := range s.ctrlConns {
		_ = c.Close()
	}
	s.connMu.Unlock()
	s.wg.Wait()
}

func (s *Modem73Simulator) acceptKISS() {
	for {
		c, err := s.kissLn.Accept()
		if err != nil {
			return
		}
		s.connMu.Lock()
		s.kissConns[c] = struct{}{}
		s.connMu.Unlock()
		go s.handleKISS(c)
	}
}

func (s *Modem73Simulator) acceptControl() {
	for {
		c, err := s.ctrlLn.Accept()
		if err != nil {
			return
		}
		s.connMu.Lock()
		s.ctrlConns[c] = struct{}{}
		s.connMu.Unlock()
		go s.handleControl(c)
	}
}

func (s *Modem73Simulator) handleKISS(c net.Conn) {
	defer func() {
		s.connMu.Lock()
		delete(s.kissConns, c)
		s.connMu.Unlock()
		_ = c.Close()
	}()
	dec := newKISSStreamDecoder(8192, func(payload []byte) {
		_ = s.transmit(payload, -1)
	})
	buf := make([]byte, 4096)
	for {
		n, err := c.Read(buf)
		if n > 0 {
			dec.feed(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (s *Modem73Simulator) handleControl(c net.Conn) {
	defer func() {
		s.connMu.Lock()
		delete(s.ctrlConns, c)
		s.connMu.Unlock()
		_ = c.Close()
	}()
	for {
		msg, err := modem73ReadControl(c)
		if err != nil {
			return
		}
		s.dispatchControl(c, msg)
	}
}

func (s *Modem73Simulator) dispatchControl(c net.Conn, msg map[string]any) {
	cmd, _ := msg["cmd"].(string)
	switch cmd {
	case "get_status":
		_ = modem73WriteControl(c, s.status())
	case "get_config":
		s.mu.Lock()
		cfg := copyMap(s.cfgMap)
		s.mu.Unlock()
		_ = modem73WriteControl(c, cfg)
	case "set_config":
		ok := s.applySetConfig(msg)
		_ = modem73WriteControl(c, map[string]any{"ok": ok})
		if ok {
			s.broadcastEvent(map[string]any{"event": "config_changed", "config": s.configSnapshot()})
		}
	case "tx":
		dataB64, _ := msg["data"].(string)
		raw, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil || len(raw) == 0 {
			_ = modem73WriteControl(c, map[string]any{"ok": false, "error": "empty or invalid base64 data"})
			return
		}
		oper := -1
		if v, ok := modem73CfgInt(msg, "oper_mode"); ok {
			oper = v
		}
		if err := s.transmit(raw, oper); err != nil {
			_ = modem73WriteControl(c, map[string]any{"ok": false, "error": "tx failed"})
			return
		}
		_ = modem73WriteControl(c, map[string]any{"ok": true, "size": float64(len(raw))})
	case "rigctl":
		_ = modem73WriteControl(c, map[string]any{"ok": true, "response": "SIM\n"})
	default:
		_ = modem73WriteControl(c, map[string]any{"ok": false, "error": "unknown command"})
	}
}

func (s *Modem73Simulator) status() map[string]any {
	st, _ := s.state.Load().(string)
	snr := math.Float64frombits(s.lastSNR.Load())
	ber := math.Float64frombits(s.lastBER.Load())
	return map[string]any{
		"ok":               true,
		"channel_state":    st,
		"ptt_on":           s.pttOn.Load(),
		"rx_frame_count":   float64(s.rxFrames.Load()),
		"tx_frame_count":   float64(s.txFrames.Load()),
		"rx_error_count":   float64(s.rxErrors.Load()),
		"sync_count":       float64(s.syncCount.Load()),
		"preamble_errors":  float64(0),
		"symbol_errors":    float64(0),
		"crc_errors":       float64(s.crcErrors.Load()),
		"last_snr":         snr,
		"last_ber":         ber,
		"ber_ema":          ber,
		"client_count":     float64(s.kissClientCount()),
		"rigctl_connected": false,
		"audio_connected":  true,
	}
}

func (s *Modem73Simulator) kissClientCount() int {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	return len(s.kissConns)
}

func (s *Modem73Simulator) configSnapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyMap(s.cfgMap)
}

func (s *Modem73Simulator) applySetConfig(msg map[string]any) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := modem73CfgInt(msg, "modem_type"); ok && v >= 0 && v <= 2 {
		s.cfg.ModemType = v
	}
	if v, ok := modem73CfgInt(msg, "robust_mode"); ok && v >= 0 && v < modem73RobustModeCount {
		s.cfg.RobustMode = v
	}
	if v, ok := modem73CfgInt(msg, "mfsk_mode"); ok && v >= 0 && v <= 3 {
		s.cfg.MFSKMode = v
	}
	if v := modem73CfgString(msg, "modulation"); v != "" {
		s.cfg.Modulation = v
	}
	if v := modem73CfgString(msg, "code_rate"); v != "" {
		s.cfg.CodeRate = v
	}
	if v, ok := modem73CfgInt(msg, "frame_size"); ok && v >= 0 && v <= 2 {
		s.cfg.FrameSize = v
	}
	if _, ok := msg["short_frame"]; ok {
		if modem73CfgBool(msg, "short_frame", false) {
			s.cfg.FrameSize = 0
		} else if s.cfg.FrameSize == 0 {
			s.cfg.FrameSize = 1
		}
	}
	if v, ok := modem73CfgInt(msg, "center_freq"); ok {
		s.cfg.CenterFreq = v
	}
	if _, ok := msg["csma_enabled"]; ok {
		s.cfg.CSMAEnabled = modem73CfgBool(msg, "csma_enabled", true)
	}
	if v, ok := modem73CfgFloat(msg, "carrier_threshold_db"); ok {
		s.cfg.CarrierThresh = v
	}
	if v, ok := modem73CfgInt(msg, "p_persistence"); ok {
		s.cfg.PPersistence = v
	}
	if v, ok := modem73CfgInt(msg, "slot_time_ms"); ok {
		s.cfg.SlotTimeMs = v
	}
	if v, ok := modem73CfgInt(msg, "csma_quiet_ms"); ok {
		s.cfg.CSMAQuietMs = v
	}
	if v, ok := modem73CfgInt(msg, "csma_cw"); ok {
		s.cfg.CSMACW = v
	}
	if v, ok := modem73CfgInt(msg, "csma_burst"); ok {
		s.cfg.CSMABurst = v
	}
	if _, ok := msg["fragmentation_enabled"]; ok {
		s.cfg.Fragmentation = modem73CfgBool(msg, "fragmentation_enabled", false)
	}
	s.rebuildConfig()
	return true
}

func (s *Modem73Simulator) transmit(payload []byte, operMode int) error {
	s.mu.Lock()
	cfg := s.cfg
	phy := Modem73ComputePhy(cfg.ModemType, cfg.RobustMode, cfg.MFSKMode, cfg.FrameSize, cfg.Modulation, cfg.CodeRate)
	if operMode >= 0 && cfg.ModemType == modem73TypeRobust && operMode < modem73RobustModeCount {
		phy = Modem73ComputePhy(modem73TypeRobust, operMode, 0, 0, "", "")
	}
	s.mu.Unlock()

	if !cfg.Fragmentation && len(payload) > phy.MTUBytes && phy.MTUBytes > 0 {
		return errors.New("payload exceeds MTU")
	}

	if cfg.CSMAEnabled {
		s.csmaWait(phy.AirtimeS)
	}

	s.pttOn.Store(true)
	s.state.Store("tx")
	air := time.Duration(phy.AirtimeS * float64(time.Second))
	if !cfg.InstantAirtime && air > 0 {
		select {
		case <-s.ctx.Done():
			s.pttOn.Store(false)
			s.state.Store("idle")
			return context.Canceled
		case <-time.After(air):
		}
	}
	s.txFrames.Add(1)
	s.ch.publish(modem73AirFrame{payload: append([]byte(nil), payload...), snrDB: s.ch.snr(), from: s}, air)
	s.pttOn.Store(false)
	s.state.Store("idle")

	s.broadcastKISS(payload)
	return nil
}

func (s *Modem73Simulator) csmaWait(airtimeS float64) {
	quietMs := float64(s.cfg.CSMAQuietMs)
	if quietMs <= 0 {
		quietMs = math.Min(math.Max(airtimeS*250, 300), 3500)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !s.ch.isBusy() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// p-persistence slot backoff
	slots := max(s.cfg.CSMACW, 2)
	p := float64(s.cfg.PPersistence) / 255.0
	for range slots {
		if s.ch.randFloat() < p {
			break
		}
		slot := time.Duration(s.cfg.SlotTimeMs) * time.Millisecond
		if s.cfg.InstantAirtime {
			continue
		}
		time.Sleep(slot)
	}
	if !s.cfg.InstantAirtime && quietMs > 0 {
		time.Sleep(time.Duration(quietMs) * time.Millisecond / 10) // scaled for tests
	}
}

func (s *Modem73Simulator) airRX() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case fr, ok := <-s.airSub:
			if !ok {
				return
			}
			if fr.from == s {
				continue
			}
			s.deliverRX(fr)
		}
	}
}

func (s *Modem73Simulator) deliverRX(fr modem73AirFrame) {
	s.syncCount.Add(1)
	s.state.Store("rx")
	defer s.state.Store("idle")

	snr := fr.snrDB
	if snr == 0 {
		snr = s.ch.snr()
	}
	// Scale SNR for higher-order OFDM roughly by bits/symbol.
	effSNR := snr
	s.mu.Lock()
	mod := s.cfg.Modulation
	ps := int(s.cfgMap["payload_size"].(float64))
	s.mu.Unlock()
	switch mod {
	case "8PSK":
		effSNR -= 3
	case "QAM16":
		effSNR -= 6
	case "QAM64":
		effSNR -= 10
	case "QAM256":
		effSNR -= 14
	case "QAM1024", "QAM4096":
		effSNR -= 18
	}
	ber := Modem73BPSKBER(effSNR)
	fer := Modem73FrameErrorRate(ber, ps)
	s.lastSNR.Store(math.Float64bits(snr))
	s.lastBER.Store(math.Float64bits(ber))

	if s.ch.randFloat() < fer {
		s.rxErrors.Add(1)
		s.crcErrors.Add(1)
		return
	}
	s.rxFrames.Add(1)
	level := -20 + snr*0.3
	s.broadcastEvent(map[string]any{
		"event":    "rx_frame",
		"snr":      snr,
		"ber_pct":  ber * 100,
		"level_db": level,
	})
	s.broadcastKISS(fr.payload)
}

func (s *Modem73Simulator) broadcastKISS(payload []byte) {
	frame := appendFrameKISS(nil, payload)
	s.connMu.Lock()
	conns := make([]net.Conn, 0, len(s.kissConns))
	for c := range s.kissConns {
		conns = append(conns, c)
	}
	s.connMu.Unlock()
	for _, c := range conns {
		_, _ = c.Write(frame)
	}
}

func (s *Modem73Simulator) broadcastEvent(msg map[string]any) {
	s.connMu.Lock()
	conns := make([]net.Conn, 0, len(s.ctrlConns))
	for c := range s.ctrlConns {
		conns = append(conns, c)
	}
	s.connMu.Unlock()
	for _, c := range conns {
		_ = modem73WriteControl(c, msg)
	}
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	maps.Copy(out, m)
	return out
}
