// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package protect provides IDS-style detect and IPS-style prevent gates for
// interface ingress handler overload stream accepts resources crypto handshakes
// adaptive rate baselines iface cool-down and memory pressure.
package protect

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/health"
)

// Decision is the result of an admit check.
type Decision struct {
	Allow  bool
	Trip   bool
	Reason Reason
}

// Options configures an Engine.
type Options struct {
	Mode                 Mode
	MaxPPS               float64
	MaxBPS               float64
	FloorPPS             float64
	FloorBPS             float64
	MaxConns             int
	MaxResources         int
	MaxCrypto            int
	MaxHandshake         int
	SoftMemoryLimit      int64
	HeapWatermark        int64
	WarnWriter           io.Writer
	WarnInterval         time.Duration
	Now                  func() time.Time
	MemorySampleFunc     func() uint64
	DisableAdaptive      bool
	DisableCoolDown      bool
	StorePath            string
	AutoLearnMinDuration time.Duration
	AutoLearnMinSamples  int
	NetworkFingerprint   string
	TransportNode        bool
}

type ifaceState struct {
	window       rateWindow
	adapt        adaptiveState
	adaptSec     int64
	adaptPeakPPS float64
	adaptPeakBPS float64
	tripAt       []time.Time
	coolUntil    time.Time
}

type warnKey struct {
	iface  string
	reason Reason
}

type warnState struct {
	last       time.Time
	suppressed uint64
}

// Engine is the process protect policy.
type Engine struct {
	mode                 Mode
	maxPPS               float64
	maxBPS               float64
	floorPPS             float64
	floorBPS             float64
	maxConns             int
	maxResources         int
	maxCrypto            int
	maxHandshake         int
	softMemoryLimit      int64
	heapWatermark        int64
	warnWriter           io.Writer
	warnInterval         time.Duration
	now                  func() time.Time
	memSample            func() uint64
	disableAdaptive      bool
	disableCoolDown      bool
	storePath            string
	autoLearnMinDuration time.Duration
	autoLearnMinSamples  int
	transportNode        bool

	mu             sync.Mutex
	ifaces         map[string]*ifaceState
	conns          map[string]int
	resources      int
	crypto         int
	handshake      int
	warns          map[warnKey]*warnState
	shedMemory     atomic.Bool
	tripCounts     [reasonCount]uint64
	autoPhase      atomic.Int32
	fingerprint    string
	promoted       bool
	learnStarted   time.Time
	stableWindows  int
	driftWindows   int
	driftSec       int64
	driftSecMaxPPS float64
	lastPersist    time.Time

	memStop chan struct{}
	memOnce sync.Once
	started atomic.Bool
}

var (
	defaultMu sync.RWMutex
	defaultE  = New(Options{Mode: ModeOff})
)

// Default returns the process-wide Engine.
func Default() *Engine {
	defaultMu.RLock()
	e := defaultE
	defaultMu.RUnlock()
	return e
}

// SetDefault installs e as the process-wide Engine. Pass nil to restore ModeOff.
func SetDefault(e *Engine) {
	defaultMu.Lock()
	if e == nil {
		defaultE = New(Options{Mode: ModeOff})
	} else {
		defaultE = e
	}
	defaultMu.Unlock()
}

// New builds an Engine from opts.
func New(opts Options) *Engine {
	if opts.MaxPPS <= 0 {
		opts.MaxPPS = DefaultMaxPPS
	}
	if opts.MaxBPS <= 0 {
		opts.MaxBPS = DefaultMaxBPS
	}
	if opts.FloorPPS <= 0 {
		opts.FloorPPS = DefaultFloorPPS
	}
	if opts.FloorBPS <= 0 {
		opts.FloorBPS = DefaultFloorBPS
	}
	if opts.MaxConns <= 0 {
		opts.MaxConns = DefaultMaxConns
	}
	if opts.MaxResources <= 0 {
		opts.MaxResources = DefaultMaxResources
	}
	if opts.MaxCrypto <= 0 {
		opts.MaxCrypto = DefaultMaxCrypto
	}
	if opts.MaxHandshake <= 0 {
		opts.MaxHandshake = DefaultMaxHandshake
	}
	if opts.HeapWatermark <= 0 {
		opts.HeapWatermark = DefaultHeapWatermarkBytes
	}
	if opts.WarnWriter == nil {
		opts.WarnWriter = os.Stdout
	}
	if opts.WarnInterval <= 0 {
		opts.WarnInterval = WarnInterval
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.MemorySampleFunc == nil {
		opts.MemorySampleFunc = func() uint64 {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			return ms.HeapAlloc
		}
	}
	if opts.AutoLearnMinDuration <= 0 {
		opts.AutoLearnMinDuration = AutoLearnMinDuration
	}
	if opts.AutoLearnMinSamples <= 0 {
		opts.AutoLearnMinSamples = AutoLearnMinSamples
	}
	e := &Engine{
		mode:                 opts.Mode,
		maxPPS:               opts.MaxPPS,
		maxBPS:               opts.MaxBPS,
		floorPPS:             opts.FloorPPS,
		floorBPS:             opts.FloorBPS,
		maxConns:             opts.MaxConns,
		maxResources:         opts.MaxResources,
		maxCrypto:            opts.MaxCrypto,
		maxHandshake:         opts.MaxHandshake,
		softMemoryLimit:      opts.SoftMemoryLimit,
		heapWatermark:        opts.HeapWatermark,
		warnWriter:           opts.WarnWriter,
		warnInterval:         opts.WarnInterval,
		now:                  opts.Now,
		memSample:            opts.MemorySampleFunc,
		disableAdaptive:      opts.DisableAdaptive,
		disableCoolDown:      opts.DisableCoolDown,
		storePath:            opts.StorePath,
		autoLearnMinDuration: opts.AutoLearnMinDuration,
		autoLearnMinSamples:  opts.AutoLearnMinSamples,
		transportNode:        opts.TransportNode,
		fingerprint:          opts.NetworkFingerprint,
		ifaces:               make(map[string]*ifaceState),
		conns:                make(map[string]int),
		warns:                make(map[warnKey]*warnState),
		memStop:              make(chan struct{}),
		learnStarted:         opts.Now(),
	}
	if opts.Mode == ModeAuto {
		e.autoPhase.Store(int32(AutoLearning))
	}
	e.loadPersisted()
	return e
}

// Mode returns the configured mode.
func (e *Engine) Mode() Mode {
	if e == nil {
		return ModeOff
	}
	return e.mode
}

// IfaceBaseline returns learned adaptive stats for iface.
func (e *Engine) IfaceBaseline(iface string) (ewmaPPS, ewmaBPS float64, samples int, ready bool) {
	if e == nil {
		return 0, 0, 0, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.ifaces[iface]
	if st == nil {
		return 0, 0, 0, false
	}
	return st.adapt.ewmaPPS, st.adapt.ewmaBPS, st.adapt.samples, st.adapt.ready
}

// TripLine returns the effective pps and bps trip limits for iface.
func (e *Engine) TripLine(iface string) (ppsLimit, bpsLimit float64) {
	if e == nil {
		return 0, 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.ifaces[iface]
	if st == nil || e.disableAdaptive {
		return e.maxPPS, e.maxBPS
	}
	return st.adapt.tripLine(e.maxPPS, e.maxBPS, e.floorPPS, e.floorBPS)
}

// StartMemoryMonitor begins periodic heap sampling when mode is not off.
func (e *Engine) StartMemoryMonitor() {
	if e == nil || e.mode == ModeOff {
		return
	}
	if !e.started.CompareAndSwap(false, true) {
		return
	}
	go e.memoryLoop()
}

// StopMemoryMonitor stops the background sampler and flushes learning state.
func (e *Engine) StopMemoryMonitor() {
	if e == nil {
		return
	}
	_ = e.Persist()
	if !e.started.Load() {
		return
	}
	e.memOnce.Do(func() {
		close(e.memStop)
	})
}

func (e *Engine) memoryLoop() {
	ticker := time.NewTicker(MemorySampleInterval)
	defer ticker.Stop()
	persistEvery := PersistInterval
	for {
		select {
		case <-e.memStop:
			return
		case <-ticker.C:
			e.ObserveMemory()
			now := e.now()
			e.mu.Lock()
			due := e.lastPersist.IsZero() || now.Sub(e.lastPersist) >= persistEvery
			e.mu.Unlock()
			if due {
				if err := e.Persist(); err == nil {
					e.mu.Lock()
					e.lastPersist = now
					e.mu.Unlock()
				}
			}
		}
	}
}

// ObserveMemory samples heap and updates the shed flag.
func (e *Engine) ObserveMemory() {
	if e == nil || e.mode == ModeOff {
		return
	}
	heap := e.memSample()
	limit := e.softMemoryLimit
	if limit <= 0 {
		limit = e.heapWatermark
	}
	if limit <= 0 {
		return
	}
	tripAt := uint64(float64(limit) * MemoryTripRatio)
	clearAt := uint64(float64(limit) * MemoryClearRatio)
	if e.shedMemory.Load() {
		if heap < clearAt {
			e.shedMemory.Store(false)
		}
		return
	}
	if heap >= tripAt {
		e.shedMemory.Store(true)
		e.recordTrip("", ReasonMemory)
	}
}

// Shedding reports whether memory pressure shed is active.
func (e *Engine) Shedding() bool {
	return e != nil && e.shedMemory.Load()
}

// InCoolDown reports whether iface is currently in cool-down.
func (e *Engine) InCoolDown(iface string) bool {
	if e == nil || e.disableCoolDown {
		return false
	}
	now := e.now()
	e.mu.Lock()
	st := e.ifaces[iface]
	ok := st != nil && now.Before(st.coolUntil)
	e.mu.Unlock()
	return ok
}

// TripCount returns how many times reason has tripped.
func (e *Engine) TripCount(reason Reason) uint64 {
	if e == nil || reason < 0 || int(reason) >= len(e.tripCounts) {
		return 0
	}
	return atomic.LoadUint64(&e.tripCounts[reason])
}

func (e *Engine) ifaceLocked(name string) *ifaceState {
	st := e.ifaces[name]
	if st == nil {
		st = &ifaceState{}
		e.ifaces[name] = st
	}
	return st
}

// AdmitPacket checks cool-down adaptive pps/bps and memory shed.
func (e *Engine) AdmitPacket(iface string, nbytes int) Decision {
	return e.admitPacket(iface, nbytes, AdmitOpts{})
}

func (e *Engine) admitPacket(iface string, nbytes int, opts AdmitOpts) Decision {
	if e == nil || e.mode == ModeOff {
		return Decision{Allow: true}
	}
	if e.shedMemory.Load() {
		return e.decide(iface, ReasonMemory)
	}
	now := e.now()
	e.mu.Lock()
	st := e.ifaceLocked(iface)
	if !e.disableCoolDown && now.Before(st.coolUntil) {
		e.mu.Unlock()
		return e.decide(iface, ReasonCoolDown)
	}
	pps, bps := st.window.add(now, nbytes)
	sampled := false
	samplePPS, sampleBPS := 0.0, 0.0
	floorPPS, floorBPS := scaledFloors(opts.Bitrate, e.floorPPS, e.floorBPS, e.maxPPS, e.maxBPS)
	if !e.disableAdaptive {
		sampled, samplePPS, sampleBPS = st.noteAdaptive(now, pps, bps)
	}
	ppsLimit, bpsLimit := e.maxPPS, e.maxBPS
	if !e.disableAdaptive {
		ppsLimit, bpsLimit = st.adapt.tripLine(e.maxPPS, e.maxBPS, floorPPS, floorBPS)
	}
	e.mu.Unlock()

	overPPS := pps > ppsLimit
	overBPS := bps > bpsLimit
	if overPPS || overBPS {
		if opts.Class.preferKeep() {
			strictPPS := ppsLimit * 2
			strictBPS := bpsLimit * 2
			if pps <= strictPPS && bps <= strictBPS {
				if sampled {
					e.maybePromoteOrDrift(iface, samplePPS, sampleBPS)
				}
				return Decision{Allow: true}
			}
		}
		e.resetDriftLocked()
		if overPPS {
			return e.tripWithCoolDown(iface, ReasonPPS)
		}
		return e.tripWithCoolDown(iface, ReasonBPS)
	}
	if sampled {
		e.maybePromoteOrDrift(iface, samplePPS, sampleBPS)
	}
	return Decision{Allow: true}
}

// noteAdaptive tracks per-second peak rates and commits one EWMA sample per second.
// Returns true when a sample was committed for the previous second.
func (st *ifaceState) noteAdaptive(now time.Time, pps, bps float64) (sampled bool, samplePPS, sampleBPS float64) {
	sec := now.Unix()
	if st.adaptSec == 0 {
		st.adaptSec = sec
		st.adaptPeakPPS = pps
		st.adaptPeakBPS = bps
		return false, 0, 0
	}
	if sec == st.adaptSec {
		if pps > st.adaptPeakPPS {
			st.adaptPeakPPS = pps
		}
		if bps > st.adaptPeakBPS {
			st.adaptPeakBPS = bps
		}
		return false, 0, 0
	}
	samplePPS = st.adaptPeakPPS
	sampleBPS = st.adaptPeakBPS
	st.adapt.observe(samplePPS, sampleBPS)
	st.adaptSec = sec
	st.adaptPeakPPS = pps
	st.adaptPeakBPS = bps
	return true, samplePPS, sampleBPS
}

func (e *Engine) resetDriftLocked() {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.driftWindows = 0
	e.mu.Unlock()
}

func (e *Engine) tripWithCoolDown(iface string, reason Reason) Decision {
	d := e.decide(iface, reason)
	if e.disableCoolDown {
		return d
	}
	now := e.now()
	e.mu.Lock()
	st := e.ifaceLocked(iface)
	cutoff := now.Add(-CoolDownTripWindow)
	kept := st.tripAt[:0]
	for _, t := range st.tripAt {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	st.tripAt = append(kept, now)
	if len(st.tripAt) >= CoolDownTripThreshold {
		st.coolUntil = now.Add(CoolDownDuration)
		st.tripAt = st.tripAt[:0]
		e.mu.Unlock()
		e.recordTrip(iface, ReasonCoolDown)
		if e.enforcementMode() == ModePrevent {
			return Decision{Allow: false, Trip: true, Reason: ReasonCoolDown}
		}
		return Decision{Allow: true, Trip: true, Reason: ReasonCoolDown}
	}
	e.mu.Unlock()
	return d
}

// AdmitHandler is called when the packet handler semaphore is full.
func (e *Engine) AdmitHandler(iface string) Decision {
	if e == nil || e.mode == ModeOff {
		return Decision{Allow: true}
	}
	return e.decide(iface, ReasonHandler)
}

// AdmitConn checks concurrent accepted connections for iface.
func (e *Engine) AdmitConn(iface string) (Decision, func()) {
	noop := func() {}
	if e == nil || e.mode == ModeOff {
		return Decision{Allow: true}, noop
	}
	if e.shedMemory.Load() {
		d := e.decide(iface, ReasonMemory)
		if !d.Allow {
			return d, noop
		}
	}
	if e.InCoolDown(iface) {
		d := e.decide(iface, ReasonCoolDown)
		if !d.Allow {
			return d, noop
		}
	}
	e.mu.Lock()
	n := e.conns[iface]
	over := n >= e.maxConns
	if over && e.enforcementMode() == ModePrevent {
		e.mu.Unlock()
		return e.decide(iface, ReasonConn), noop
	}
	e.conns[iface] = n + 1
	e.mu.Unlock()
	var trip Decision
	if over {
		trip = e.decide(iface, ReasonConn)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			e.mu.Lock()
			if e.conns[iface] > 0 {
				e.conns[iface]--
			}
			e.mu.Unlock()
		})
	}
	if over {
		trip.Allow = true
		return trip, release
	}
	return Decision{Allow: true}, release
}

// AdmitResource checks concurrent incoming resources and memory shed.
func (e *Engine) AdmitResource(estBytes int64) (Decision, func()) {
	noop := func() {}
	if e == nil || e.mode == ModeOff {
		return Decision{Allow: true}, noop
	}
	_ = estBytes
	if e.shedMemory.Load() {
		d := e.decide("", ReasonMemory)
		if !d.Allow {
			return d, noop
		}
	}
	e.mu.Lock()
	over := e.resources >= e.maxResources
	if over && e.enforcementMode() == ModePrevent {
		e.mu.Unlock()
		return e.decide("", ReasonResource), noop
	}
	e.resources++
	e.mu.Unlock()
	var trip Decision
	if over {
		trip = e.decide("", ReasonResource)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			e.mu.Lock()
			if e.resources > 0 {
				e.resources--
			}
			e.mu.Unlock()
		})
	}
	if over {
		trip.Allow = true
		return trip, release
	}
	return Decision{Allow: true}, release
}

// AdmitCrypto gates concurrent decrypt and signature verify work.
func (e *Engine) AdmitCrypto(iface string) (Decision, func()) {
	return e.admitSlot(iface, ReasonCrypto, &e.crypto, e.maxCrypto)
}

// AdmitHandshake gates concurrent link handshake work.
func (e *Engine) AdmitHandshake(iface string) (Decision, func()) {
	return e.admitSlot(iface, ReasonHandshake, &e.handshake, e.maxHandshake)
}

func (e *Engine) admitSlot(iface string, reason Reason, slot *int, max int) (Decision, func()) {
	noop := func() {}
	if e == nil || e.mode == ModeOff {
		return Decision{Allow: true}, noop
	}
	if e.shedMemory.Load() {
		d := e.decide(iface, ReasonMemory)
		if !d.Allow {
			return d, noop
		}
	}
	e.mu.Lock()
	over := *slot >= max
	if over && e.enforcementMode() == ModePrevent {
		e.mu.Unlock()
		return e.decide(iface, reason), noop
	}
	*slot++
	e.mu.Unlock()
	var trip Decision
	if over {
		trip = e.decide(iface, reason)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			e.mu.Lock()
			if *slot > 0 {
				*slot--
			}
			e.mu.Unlock()
		})
	}
	if over {
		trip.Allow = true
		return trip, release
	}
	return Decision{Allow: true}, release
}

func (e *Engine) decide(iface string, reason Reason) Decision {
	e.recordTrip(iface, reason)
	if e.enforcementMode() == ModePrevent {
		return Decision{Allow: false, Trip: true, Reason: reason}
	}
	return Decision{Allow: true, Trip: true, Reason: reason}
}

func (e *Engine) recordTrip(iface string, reason Reason) {
	if reason > ReasonNone && int(reason) < len(e.tripCounts) {
		atomic.AddUint64(&e.tripCounts[reason], 1)
	}
	health.Inc(iface, reason.HealthKind())
	e.warn(iface, reason)
}

func (e *Engine) warn(iface string, reason Reason) {
	now := e.now()
	key := warnKey{iface: iface, reason: reason}
	e.mu.Lock()
	st := e.warns[key]
	if st == nil {
		st = &warnState{}
		e.warns[key] = st
	}
	elapsed := now.Sub(st.last)
	if !st.last.IsZero() && elapsed < e.warnInterval {
		st.suppressed++
		e.mu.Unlock()
		return
	}
	suppressed := st.suppressed
	st.suppressed = 0
	st.last = now
	writer := e.warnWriter
	modeLabel := e.mode.String()
	if e.mode == ModeAuto {
		modeLabel = "auto/" + e.Phase().String()
	}
	e.mu.Unlock()

	ifacePart := iface
	if ifacePart == "" {
		ifacePart = "-"
	}
	line := fmt.Sprintf("WARNING: dos_protection %s trip reason=%s iface=%s", modeLabel, reason.String(), ifacePart)
	if suppressed > 0 {
		line = fmt.Sprintf("%s suppressed=%d", line, suppressed)
	}
	_, _ = fmt.Fprintln(writer, line)
}

// Package-level helpers use Default().

// AdmitPacket checks the default engine.
func AdmitPacket(iface string, nbytes int) Decision {
	return Default().AdmitPacket(iface, nbytes)
}

// AdmitPacketOpts checks the default engine with bitrate and packet class.
func AdmitPacketOpts(iface string, nbytes int, opts AdmitOpts) Decision {
	return Default().admitPacket(iface, nbytes, opts)
}

// AdmitHandler checks the default engine.
func AdmitHandler(iface string) Decision {
	return Default().AdmitHandler(iface)
}

// AdmitConn checks the default engine.
func AdmitConn(iface string) (Decision, func()) {
	return Default().AdmitConn(iface)
}

// AdmitResource checks the default engine.
func AdmitResource(estBytes int64) (Decision, func()) {
	return Default().AdmitResource(estBytes)
}

// AdmitCrypto checks the default engine.
func AdmitCrypto(iface string) (Decision, func()) {
	return Default().AdmitCrypto(iface)
}

// AdmitHandshake checks the default engine.
func AdmitHandshake(iface string) (Decision, func()) {
	return Default().AdmitHandshake(iface)
}

// ConfigureFromConfig installs a default engine from dos_protection soft memory limit and optional store path.
func ConfigureFromConfig(modeStr string, softMemoryLimit int64, storePath string, cfg *common.ReticulumConfig) *Engine {
	mode, ok := ParseMode(modeStr)
	if !ok {
		mode = ModeOff
	}
	opts := Options{
		Mode:            mode,
		SoftMemoryLimit: softMemoryLimit,
		StorePath:       storePath,
	}
	if cfg != nil {
		opts.TransportNode = cfg.EnableTransport
		if cfg.EnableTransport {
			opts.AutoLearnMinDuration = AutoLearnMinDuration * 2
		}
		if cfg.DoSMaxPPS > 0 {
			opts.MaxPPS = cfg.DoSMaxPPS
		}
		if cfg.DoSMaxBPS > 0 {
			opts.MaxBPS = cfg.DoSMaxBPS
		}
		if cfg.DoSFloorPPS > 0 {
			opts.FloorPPS = cfg.DoSFloorPPS
		}
		if cfg.DoSFloorBPS > 0 {
			opts.FloorBPS = cfg.DoSFloorBPS
		}
		if cfg.DoSMaxConns > 0 {
			opts.MaxConns = cfg.DoSMaxConns
		}
		if cfg.DoSMaxResources > 0 {
			opts.MaxResources = cfg.DoSMaxResources
		}
		if cfg.DoSMaxCrypto > 0 {
			opts.MaxCrypto = cfg.DoSMaxCrypto
		}
		if cfg.DoSMaxHandshake > 0 {
			opts.MaxHandshake = cfg.DoSMaxHandshake
		}
	}
	e := New(opts)
	SetDefault(e)
	if mode != ModeOff {
		e.StartMemoryMonitor()
	}
	return e
}
