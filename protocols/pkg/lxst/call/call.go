// SPDX-License-Identifier: LicenseRef-Reticulum
package call

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/audio/filter"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/audio/io"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/audio/opus"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/audio/opusfile"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/audio/tone"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/media"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/proto"
)

var (
	ErrNotActive       = errors.New("call not active")
	ErrAlreadyCall     = errors.New("call already in progress")
	ErrNoIdentity      = errors.New("local identity required")
	ErrRemoteRequired  = errors.New("remote identity required")
	ErrNotRinging      = errors.New("call not ringing")
	ErrNotIncoming     = errors.New("not an incoming call")
	ErrInvalidMode     = errors.New("invalid call mode")
	ErrPipelinesClosed = errors.New("pipelines not open")
	ErrCallEnded       = errors.New("call ended before becoming active")
	ErrStateTimeout    = errors.New("timeout waiting for call state")
	ErrAnswerRaced     = errors.New("call no longer ringing")
	ErrBlocked         = errors.New("caller blocked")
	ErrRateLimited     = errors.New("rate limited")
	ErrInvalidProfile  = errors.New("invalid call profile")
)

type State int32

const (
	StateIdle State = iota
	StateRinging
	StateConnecting
	StateActive
	StateEnded
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateRinging:
		return "ringing"
	case StateConnecting:
		return "connecting"
	case StateActive:
		return "active"
	case StateEnded:
		return "ended"
	default:
		return "unknown"
	}
}

type Events struct {
	OnRinging  func(*Call)
	OnAnswered func(*Call)
	OnBusy     func(*Call)
	OnRejected func(*Call)
	OnStats    func(*Call, media.LinkMetrics)
	OnFrame    func([]int16)
	OnEnded    func(*Call, string)
}

type Config struct {
	AppName          string
	AspectName       string
	Identity         *identity.Identity
	Events           Events
	UseAudio         bool
	Device           io.Device
	DuplexIO         bool
	Profile          int
	Mode             int
	RingTime         time.Duration
	WaitTime         time.Duration
	ConnectTime      time.Duration
	AutoAnswer       time.Duration
	AnnounceInterval time.Duration
	AllowPolicy      byte
	Allowed          [][]byte
	Blocked          [][]byte
	AllowFunc        func([]byte) bool
	TransmitGainDB   float64
	ReceiveGainDB    float64
	Filters          bool
	RingtonePath     string
	RingtoneGain     float64
	Speaker          string
	Microphone       string
	Ringer           string
}

type Call struct {
	cfg       Config
	transport *transport.Transport
	events    Events

	state         atomic.Int32
	muted         atomic.Bool
	mutedRX       atomic.Bool
	squelched     atomic.Bool
	incoming      atomic.Bool
	answered      atomic.Bool
	identified    atomic.Bool
	modeFollowOff atomic.Bool
	status        atomic.Int32
	profile       atomic.Int32
	callMode      atomic.Int32
	recvCodec     atomic.Uint32
	frameSeq      atomic.Uint32
	recvCount     atomic.Uint64
	sentCount     atomic.Uint64
	ringPos       atomic.Uint64
	txGainBits    atomic.Uint64
	rxGainBits    atomic.Uint64
	peer          atomic.Pointer[link.Link]

	jitter   *media.JitterBuffer
	adaptive *media.AdaptiveController
	bandpass *filter.BandPass
	agc      *filter.AGC
	echo     *filter.EchoSuppressor
	ringA    *tone.Source
	ringB    *tone.Source
	dial     *tone.Source
	limiter  *RateLimiter

	mutex       sync.Mutex
	sigMu       sync.Mutex
	ringOnce    sync.Once
	ringLoad    sync.Once
	ringStop    chan struct{}
	ringWG      sync.WaitGroup
	ringClip    *opusfile.Clip
	encoder     opus.Encoder
	decoder     opus.Decoder
	device      io.Device
	deviceOwned bool
	params      proto.CodecParams
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	stateWait   chan struct{}
	recvWake    chan struct{}
	txArm       chan struct{}
	txArmed     atomic.Bool
	opened      bool
	started     bool
	endOnce     sync.Once
	ended       chan struct{}
	endErr      error
	recvKind    byte
	skipLeft    int
	easeLeft    int
	easeTotal   int
}

func DefaultConfig() Config {
	return Config{
		UseAudio: true,
		DuplexIO: true,
		Filters:  true,
	}.withDefaults()
}

func NewCall(t *transport.Transport, cfg Config) *Call {
	cfg = cfg.withDefaults()
	c := &Call{
		cfg:       cfg,
		transport: t,
		events:    cfg.Events,
		jitter:    media.NewJitterBuffer(defaultJitterTargetMs, defaultJitterMaxFrames),
		ringStop:  make(chan struct{}),
		stateWait: make(chan struct{}, 1),
		recvWake:  make(chan struct{}, 1),
		txArm:     make(chan struct{}, 1),
		ended:     make(chan struct{}),
		adaptive:  media.NewAdaptiveController(),
		bandpass:  filter.NewBandPass(speechLowHz, speechHighHz, io.DefaultSampleRate),
		agc:       filter.NewAGC(defaultAGCdB),
		echo:      filter.NewEchoSuppressor(),
		ringA:     tone.New(tone.RingAHz, io.DefaultSampleRate, ringGain),
		ringB:     tone.New(tone.RingBHz, io.DefaultSampleRate, ringGain),
		dial:      tone.New(tone.DialHz, io.DefaultSampleRate, dialGain),
	}
	c.state.Store(int32(StateIdle))
	c.status.Store(int32(proto.StatusAvailable))
	c.callMode.Store(clampInt32(cfg.Mode))
	c.txGainBits.Store(math.Float64bits(cfg.TransmitGainDB))
	c.rxGainBits.Store(math.Float64bits(cfg.ReceiveGainDB))
	if cfg.Mode == proto.ModeHalfDuplex {
		c.squelched.Store(true)
	}
	c.applyProfile(cfg.Profile)
	return c
}

func (c *Call) State() State {
	return State(c.state.Load())
}

func (c *Call) Status() int {
	return int(c.status.Load())
}

func (c *Call) Profile() int {
	return int(c.profile.Load())
}

func (c *Call) Mode() int {
	return int(c.callMode.Load())
}

func (c *Call) Transmitting() bool {
	if c.state.Load() != int32(StateActive) || c.muted.Load() {
		return false
	}
	if c.callMode.Load() == int32(proto.ModeHalfDuplex) && c.squelched.Load() {
		return false
	}
	return true
}

func (c *Call) Squelched() bool {
	return c.squelched.Load()
}

func (c *Call) Incoming() bool {
	return c.incoming.Load()
}

func (c *Call) RecvFrames() uint64 {
	return c.recvCount.Load()
}

func (c *Call) SentFrames() uint64 {
	return c.sentCount.Load()
}

func (c *Call) RemoteIdentity() *identity.Identity {
	l := c.getLink()
	if l == nil {
		return nil
	}
	return l.GetRemoteIdentity()
}

func (c *Call) getLink() *link.Link {
	return c.peer.Load()
}

func (c *Call) setLink(l *link.Link) {
	c.peer.Store(l)
}

func (c *Call) SetRateCheck(l *RateLimiter) {
	c.limiter = l
}

// Dial places an outgoing call and blocks until the call is Active or the
// wait fails. OnAnswered may still be running when Dial returns. Time first
// media from OnAnswered or OnFrame, not from Dial returning.
func (c *Call) Dial(ctx context.Context, remote *identity.Identity) error {
	if remote == nil {
		return ErrRemoteRequired
	}
	if c.cfg.Identity == nil {
		return ErrNoIdentity
	}
	if !c.casState(StateIdle, StateConnecting) {
		return ErrAlreadyCall
	}
	c.incoming.Store(false)
	c.status.Store(int32(proto.StatusCalling))

	outDest, err := destination.New(remote, destination.Out, destination.Single, c.cfg.AppName, c.transport, c.cfg.AspectName)
	if err != nil {
		c.storeState(StateIdle)
		return err
	}

	l := link.NewLink(outDest, c.transport, nil, func(lnk *link.Link) {
		c.onOutgoingEstablished(lnk)
	}, c.onLinkClosed)
	l.SetPacketCallback(c.onPacket)
	c.setLink(l)
	if err := l.Establish(); err != nil {
		c.storeState(StateIdle)
		c.setLink(nil)
		return err
	}
	l.Start()
	c.armTimeout(c.cfg.WaitTime, func() bool {
		return c.status.Load() < int32(proto.StatusEstablished)
	}, "call timeout")
	return c.waitForState(ctx, StateActive, c.cfg.WaitTime)
}

func (c *Call) ServeIncoming(l *link.Link) error {
	if !c.casState(StateIdle, StateConnecting) {
		return ErrAlreadyCall
	}
	c.incoming.Store(true)
	c.setLink(l)
	l.SetPacketCallback(c.onPacket)
	l.SetRemoteIdentifiedCallback(c.onIdentified)
	l.SetLinkClosedCallback(c.onLinkClosed)
	c.status.Store(int32(proto.StatusAvailable))
	if err := c.sendSignals(proto.StatusAvailable); err != nil {
		c.storeState(StateIdle)
		c.setLink(nil)
		return err
	}
	go c.resendAvailableUntilIdentified()
	c.armTimeout(c.cfg.ConnectTime, func() bool {
		return c.incoming.Load() && c.state.Load() == int32(StateConnecting)
	}, "identify timeout")
	return nil
}

func (c *Call) Answer(_ context.Context) error {
	if c.state.Load() != int32(StateRinging) {
		return ErrNotRinging
	}
	if !c.incoming.Load() {
		return ErrNotIncoming
	}
	c.answered.Store(true)
	c.stopRingtone()
	if err := c.sendSignals(proto.StatusConnecting); err != nil {
		return err
	}
	if err := c.openPipelines(); err != nil {
		return err
	}
	if !c.casState(StateRinging, StateActive) {
		c.end("answer raced hangup")
		return ErrAnswerRaced
	}
	c.status.Store(int32(proto.StatusEstablished))
	if err := c.startPipelines(); err != nil {
		c.end("pipeline start failed")
		return err
	}
	if err := c.sendSignals(proto.StatusEstablished); err != nil {
		c.end("establish failed")
		return err
	}
	c.armTX()
	if c.events.OnAnswered != nil {
		c.events.OnAnswered(c)
	}
	return nil
}

func (c *Call) Reject(reason string) error {
	if c.state.Load() != int32(StateRinging) {
		return ErrNotRinging
	}
	if c.incoming.Load() {
		_ = c.sendSignals(proto.StatusRejected)
		time.Sleep(rejectFlushDelay)
	}
	if reason == "" {
		reason = "rejected"
	}
	c.end(reason)
	return nil
}

func (c *Call) Hangup(reason string) error {
	st := c.state.Load()
	if st == int32(StateIdle) || st == int32(StateEnded) {
		return nil
	}
	if c.incoming.Load() && st == int32(StateRinging) && !c.answered.Load() {
		_ = c.sendSignals(proto.StatusRejected)
	}
	c.end(reason)
	return nil
}

func (c *Call) SetTransmitGain(db float64) {
	c.txGainBits.Store(math.Float64bits(db))
}

func (c *Call) SetReceiveGain(db float64) {
	c.rxGainBits.Store(math.Float64bits(db))
}

func (c *Call) SetRingtone(path string, gainDB float64) {
	c.mutex.Lock()
	c.cfg.RingtonePath = path
	c.cfg.RingtoneGain = gainDB
	c.mutex.Unlock()
}

func (c *Call) SetSpeaker(name string) {
	c.mutex.Lock()
	c.cfg.Speaker = name
	c.mutex.Unlock()
}

func (c *Call) SetMicrophone(name string) {
	c.mutex.Lock()
	c.cfg.Microphone = name
	c.mutex.Unlock()
}

func (c *Call) SetRinger(name string) {
	c.mutex.Lock()
	c.cfg.Ringer = name
	c.mutex.Unlock()
}

func (c *Call) Mute(muted bool) error {
	return c.MuteTX(muted)
}

func (c *Call) MuteTX(muted bool) error {
	if !c.isLive() {
		return ErrNotActive
	}
	c.muted.Store(muted)
	return nil
}

func (c *Call) MutedTX() bool {
	return c.muted.Load()
}

func (c *Call) MutedRX() bool {
	return c.mutedRX.Load()
}

func (c *Call) MuteRX(muted bool) error {
	if !c.isLive() {
		return ErrNotActive
	}
	c.mutedRX.Store(muted)
	return nil
}

func (c *Call) isLive() bool {
	st := c.state.Load()
	return st == int32(StateActive) || st == int32(StateRinging)
}

func (c *Call) setSquelchState(on bool) {
	if c.squelched.Load() == on {
		return
	}
	c.squelched.Store(on)
	if c.state.Load() == int32(StateActive) {
		if on {
			c.PauseAGC()
		} else {
			c.ResumeAGC()
		}
	}
}

func (c *Call) applyModeSquelch() {
	c.setSquelchState(c.callMode.Load() == int32(proto.ModeHalfDuplex))
}

func (c *Call) Squelch(on bool) {
	if c.state.Load() != int32(StateActive) {
		return
	}
	c.setSquelchState(on)
}

func (c *Call) PTT(down bool) {
	if c.state.Load() != int32(StateActive) {
		return
	}
	c.setSquelchState(!down)
}

func (c *Call) DisableRemoteModeFollow() {
	c.modeFollowOff.Store(true)
}

func (c *Call) SwitchProfile(profile int) error {
	if profile == 0 {
		profile = proto.DefaultProfile
	}
	if !proto.KnownProfile(profile) {
		return ErrInvalidProfile
	}
	if c.state.Load() == int32(StateActive) {
		if err := c.switchProfile(profile); err != nil {
			return err
		}
		return c.sendSignals(proto.SignalPreferredProfile(profile))
	}
	c.applyProfile(profile)
	return nil
}

func (c *Call) EndError() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.endErr
}

func (c *Call) SwitchMode(mode int) error {
	if mode != proto.ModeFullDuplex && mode != proto.ModeHalfDuplex {
		return ErrInvalidMode
	}
	c.callMode.Store(int32(mode))
	if c.state.Load() == int32(StateActive) {
		c.setSquelchState(mode == proto.ModeHalfDuplex)
		return c.sendSignals(proto.SignalPreferredMode(mode))
	}
	c.squelched.Store(mode == proto.ModeHalfDuplex)
	return nil
}

func (c *Call) PauseAGC() {
	if c.agc != nil {
		c.agc.Pause()
	}
}

func (c *Call) ResumeAGC() {
	if c.agc != nil {
		c.agc.Resume()
	}
}

func (c *Call) waitForState(ctx context.Context, want State, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(statePollInterval)
	defer tick.Stop()
	for {
		st := c.state.Load()
		if st == int32(want) {
			return nil
		}
		if st == int32(StateEnded) {
			return ErrCallEnded
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return ErrStateTimeout
		case <-c.stateWait:
		case <-tick.C:
		}
	}
}

func (c *Call) casState(from, to State) bool {
	if !c.state.CompareAndSwap(int32(from), int32(to)) {
		return false
	}
	c.kickWaiters()
	return true
}

func (c *Call) storeState(s State) {
	c.state.Store(int32(s))
	c.kickWaiters()
}

func (c *Call) kickWaiters() {
	if c.stateWait == nil {
		return
	}
	select {
	case c.stateWait <- struct{}{}:
	default:
	}
}

func (c *Call) wakeRecv() {
	if c.recvWake == nil {
		return
	}
	select {
	case c.recvWake <- struct{}{}:
	default:
	}
}

func (c *Call) armTX() {
	c.txArmed.Store(true)
	if c.txArm == nil {
		return
	}
	select {
	case c.txArm <- struct{}{}:
	default:
	}
}

func (c *Call) sleepEnded(d time.Duration) bool {
	if d <= 0 {
		return c.endedClosed()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return c.endedClosed()
	case <-c.ended:
		return true
	}
}

func (c *Call) endedClosed() bool {
	select {
	case <-c.ended:
		return true
	default:
		return false
	}
}

func (c *Call) armTimeout(d time.Duration, stillWaiting func() bool, reason string) {
	if d <= 0 {
		return
	}
	go func() {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-timer.C:
			if stillWaiting() && c.state.Load() != int32(StateEnded) && c.state.Load() != int32(StateIdle) {
				c.end(reason)
			}
		case <-c.ended:
		}
	}()
}

func (c *Call) end(reason string) {
	c.endOnce.Do(func() {
		if c.ended != nil {
			close(c.ended)
		}
	})
	if !c.casState(StateActive, StateEnded) &&
		!c.casState(StateRinging, StateEnded) &&
		!c.casState(StateConnecting, StateEnded) {
		if c.state.Load() == int32(StateEnded) {
			return
		}
		c.storeState(StateEnded)
	}
	c.status.Store(int32(proto.StatusAvailable))
	c.stopRingtone()

	c.mutex.Lock()
	cancel := c.cancel
	c.cancel = nil
	c.mutex.Unlock()
	l := c.getLink()

	if cancel != nil {
		cancel()
	}
	c.wg.Wait()

	c.mutex.Lock()
	encoder := c.encoder
	decoder := c.decoder
	device := c.device
	c.encoder = nil
	c.decoder = nil
	c.device = nil
	owned := c.deviceOwned
	c.deviceOwned = false
	c.opened = false
	c.started = false
	c.mutex.Unlock()
	if encoder != nil {
		_ = encoder.Close()
	}
	if decoder != nil {
		_ = decoder.Close()
	}
	if device != nil && owned {
		_ = device.Close()
	}
	if l != nil && l.IsActive() {
		l.Teardown()
	}
	if c.events.OnEnded != nil {
		c.events.OnEnded(c, reason)
	}
}

func clampInt32(v int) int32 {
	if v > math.MaxInt32 {
		v = math.MaxInt32
	}
	if v < math.MinInt32 {
		v = math.MinInt32
	}
	return int32(v) // #nosec G115 -- clamped to int32 range
}
