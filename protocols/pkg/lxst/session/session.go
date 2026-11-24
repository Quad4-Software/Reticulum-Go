// SPDX-License-Identifier: LicenseRef-Reticulum

// Package session is a host-PCM telephone API for LXST calls.
//
// Peers are identity and destination hashes, not hosts or URLs. The app
// supplies a Reticulum transport. This package does not open sockets onto
// the mesh and does not terminate LXST on a server.
//
// Announce is explicit. Ringing is not trust. Show the caller fingerprint
// from OnRinging or Info.RemoteHash, then Answer or AnswerHash.
//
// Attach copies 48 kHz mono PCM16 on a local stream. That stream is not a
// Reticulum hop and is not encrypted by this package. Do not expose it as
// a public network listener.
//
// Info, Events.OnState, Events.OnError, and Config.Log are local diagnostics.
// They do not change the LXST wire format.
package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/audio/io"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/call"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/media"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/phonebook"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/proto"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/rnsnode"
)

const defaultRecall = 10 * time.Second

type LogFunc func(event string, kv ...string)

type Events struct {
	OnRinging  func(remote *identity.Identity, incoming bool)
	OnAnswered func()
	OnBusy     func()
	OnRejected func()
	OnEnded    func(reason string)
	OnStats    func(media.LinkMetrics)
	OnPCM      func([]int16)
	OnState    func(Info)
	OnError    func(error)
}

type Config struct {
	Transport        *transport.Transport
	Identity         *identity.Identity
	Destination      *destination.Destination
	Device           io.Device
	Profile          int
	Mode             int
	RecallTimeout    time.Duration
	AnnounceInterval time.Duration
	AllowPolicy      byte
	Allowed          [][]byte
	Blocked          [][]byte
	AllowFunc        func([]byte) bool
	Events           Events
	Log              LogFunc
	NoRateLimit      bool
	RateInterval     time.Duration
	RateBurst        int
}

type Session struct {
	cfg    Config
	phone  *call.Phone
	host   *io.Host
	owned  bool
	recall time.Duration
	logFn  LogFunc

	mutex     sync.Mutex
	events    Events
	lastErr   error
	reason    string
	announced bool
}

func Open(cfg Config) (*Session, error) {
	if cfg.Transport == nil {
		return nil, ErrNoTransport
	}
	if cfg.Identity == nil {
		return nil, ErrNoIdentity
	}
	recall := cfg.RecallTimeout
	if recall <= 0 {
		recall = defaultRecall
	}
	s := &Session{cfg: cfg, recall: recall, events: cfg.Events, logFn: cfg.Log}
	dev := cfg.Device
	if dev == nil {
		s.host = io.NewHost()
		dev = s.host
		s.owned = true
	} else if h, ok := dev.(*io.Host); ok {
		s.host = h
	}
	if s.host != nil && s.events.OnPCM != nil {
		s.host.SetPlaybackHandler(s.events.OnPCM)
	}
	dest := cfg.Destination
	if dest == nil {
		var err error
		dest, err = destination.New(cfg.Identity, destination.In, destination.Single, proto.AppName, cfg.Transport, proto.AspectName)
		if err != nil {
			return nil, fmt.Errorf("telephony destination: %w", err)
		}
	}
	phoneCfg := call.Config{
		Identity:         cfg.Identity,
		Device:           dev,
		UseAudio:         true,
		DuplexIO:         true,
		Profile:          cfg.Profile,
		Mode:             cfg.Mode,
		AnnounceInterval: cfg.AnnounceInterval,
		AllowPolicy:      cfg.AllowPolicy,
		Allowed:          cfg.Allowed,
		Blocked:          cfg.Blocked,
		AllowFunc:        cfg.AllowFunc,
		Events:           s.callEvents(),
	}
	s.phone = call.NewPhone(cfg.Transport, dest, phoneCfg)
	if !cfg.NoRateLimit {
		s.phone.Switchboard().SetRateLimiter(call.NewRateLimiter(cfg.RateInterval, cfg.RateBurst))
	}
	profile := phoneCfg.Profile
	if profile == 0 {
		profile = proto.DefaultProfile
	}
	mode := phoneCfg.Mode
	if mode == 0 {
		mode = proto.DefaultMode
	}
	rate := "on"
	if cfg.NoRateLimit {
		rate = "off"
	}
	s.note("open",
		"dest", call.FormatHash(s.phone.DestHash()),
		"aspect", proto.AppName+"."+proto.AspectName,
		"audio", s.audioKind(),
		"profile", proto.ProfileName(profile),
		"mode", proto.ModeName(mode),
		"allow", policyName(phoneCfg.AllowPolicy, phoneCfg.Allowed, phoneCfg.AllowFunc),
		"announce", "manual",
		"rate_limit", rate,
	)
	return s, nil
}

// Phone returns the underlying LXST phone for protocol-level work.
func (s *Session) Phone() *call.Phone {
	return s.phone
}

func (s *Session) Host() *io.Host {
	return s.host
}

func (s *Session) DestHash() []byte {
	return s.phone.DestHash()
}

func (s *Session) Active() *call.Call {
	return s.phone.Switchboard().Active()
}

func (s *Session) Announce() error {
	err := s.phone.Announce()
	if err != nil {
		return s.fail(fmt.Errorf("announce: %w", err))
	}
	s.mutex.Lock()
	s.announced = true
	s.mutex.Unlock()
	s.note("announce", "dest", call.FormatHash(s.phone.DestHash()))
	return nil
}

func (s *Session) StartAnnounce(ctx context.Context) {
	s.phone.StartAnnounce(ctx)
	s.mutex.Lock()
	s.announced = true
	s.mutex.Unlock()
	s.note("announce_loop", "dest", call.FormatHash(s.phone.DestHash()))
}

func (s *Session) SetBusy(busy bool) {
	s.phone.SetBusy(busy)
	s.note("busy", "on", boolField(busy))
}

func (s *Session) SetAllowed(policy byte, hashes [][]byte, fn func([]byte) bool) {
	s.mutex.Lock()
	s.cfg.AllowPolicy = policy
	s.cfg.Allowed = hashes
	s.cfg.AllowFunc = fn
	s.mutex.Unlock()
	s.phone.SetAllowed(policy, hashes, fn)
}

func (s *Session) SetBlocked(hashes [][]byte) {
	s.phone.SetBlocked(hashes)
}

func (s *Session) SetOnPCM(fn func([]int16)) {
	s.mutex.Lock()
	s.events.OnPCM = fn
	s.mutex.Unlock()
	if s.host != nil {
		s.host.SetPlaybackHandler(fn)
	}
}

func (s *Session) Dial(ctx context.Context, remote *identity.Identity) (*call.Call, error) {
	if remote == nil {
		return nil, s.fail(fmt.Errorf("dial: %w", call.ErrRemoteRequired))
	}
	s.note("dial", "remote", call.Fingerprint(remote))
	s.emitState()
	c, err := s.phone.Dial(ctx, remote)
	if err != nil {
		return nil, s.fail(fmt.Errorf("dial: %w", err))
	}
	s.emitState()
	return c, nil
}

func (s *Session) DialHash(ctx context.Context, destHex string) (*call.Call, error) {
	raw, err := ParseHash(destHex)
	if err != nil {
		return nil, s.fail(fmt.Errorf("dial: %w", err))
	}
	candidates := [][]byte{raw}
	if len(raw) == proto.IdentityHashLen {
		candidates = append(candidates, proto.TelephonyHash(raw))
	}
	s.note("recall", "hash", call.FormatHash(raw))
	remote, err := rnsnode.WaitRecallContext(ctx, s.cfg.Transport, candidates, s.recall)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, s.fail(fmt.Errorf("dial: %w", err))
		}
		return nil, s.fail(fmt.Errorf("%w: %s", ErrRecall, call.FormatHash(raw)))
	}
	return s.Dial(ctx, remote)
}

// GoDial starts Dial without blocking. Failures go to OnError and LastError.
func (s *Session) GoDial(ctx context.Context, remote *identity.Identity) {
	go func() {
		_, _ = s.Dial(ctx, remote)
	}()
}

func (s *Session) GoDialHash(ctx context.Context, destHex string) {
	go func() {
		_, _ = s.DialHash(ctx, destHex)
	}()
}

func (s *Session) Answer(ctx context.Context) error {
	c := s.Active()
	if c == nil {
		return s.fail(fmt.Errorf("answer: %w", call.ErrNotRinging))
	}
	s.note("answer", "remote", call.Fingerprint(c.RemoteIdentity()))
	if err := c.Answer(ctx); err != nil {
		return s.fail(fmt.Errorf("answer: %w", err))
	}
	s.emitState()
	return nil
}

// AnswerHash answers only if the caller identity or telephony hash matches want.
func (s *Session) AnswerHash(ctx context.Context, want string) error {
	c := s.Active()
	if c == nil {
		return s.fail(fmt.Errorf("answer: %w", call.ErrNotRinging))
	}
	raw, err := ParseHash(want)
	if err != nil {
		return s.fail(fmt.Errorf("answer: %w", err))
	}
	remote := c.RemoteIdentity()
	if remote == nil {
		return s.fail(fmt.Errorf("answer: %w", call.ErrNotRinging))
	}
	idHash := remote.Hash()
	if !bytes.Equal(raw, idHash) && !bytes.Equal(raw, proto.TelephonyHash(idHash)) {
		return s.fail(fmt.Errorf("answer: %w", ErrFingerprint))
	}
	return s.Answer(ctx)
}

func (s *Session) Reject() error {
	c := s.Active()
	if c == nil {
		return s.fail(fmt.Errorf("reject: %w", call.ErrNotRinging))
	}
	s.note("reject", "remote", call.Fingerprint(c.RemoteIdentity()))
	if err := c.Reject("rejected"); err != nil {
		return s.fail(fmt.Errorf("reject: %w", err))
	}
	s.emitState()
	return nil
}

func (s *Session) Hangup() error {
	c := s.Active()
	if c == nil {
		return nil
	}
	s.note("hangup", "remote", call.Fingerprint(c.RemoteIdentity()), "state", c.State().String())
	if err := c.Hangup("hangup"); err != nil {
		return s.fail(fmt.Errorf("hangup: %w", err))
	}
	s.emitState()
	return nil
}

func (s *Session) MuteTX(muted bool) error {
	c := s.Active()
	if c == nil {
		return ErrNoCall
	}
	if err := c.MuteTX(muted); err != nil {
		return s.fail(fmt.Errorf("mute tx: %w", err))
	}
	s.note("mute_tx", "on", boolField(muted))
	s.emitState()
	return nil
}

func (s *Session) MuteRX(muted bool) error {
	c := s.Active()
	if c == nil {
		return ErrNoCall
	}
	if err := c.MuteRX(muted); err != nil {
		return s.fail(fmt.Errorf("mute rx: %w", err))
	}
	s.note("mute_rx", "on", boolField(muted))
	s.emitState()
	return nil
}

func (s *Session) PTT(down bool) error {
	c := s.Active()
	if c == nil {
		return ErrNoCall
	}
	c.PTT(down)
	s.note("ptt", "down", boolField(down))
	s.emitState()
	return nil
}

func (s *Session) SwitchProfile(profile int) error {
	c := s.Active()
	if c == nil {
		return ErrNoCall
	}
	if err := c.SwitchProfile(profile); err != nil {
		return s.fail(fmt.Errorf("profile: %w", err))
	}
	s.note("profile", "name", proto.ProfileName(c.Profile()))
	s.emitState()
	return nil
}

func (s *Session) SwitchMode(mode int) error {
	c := s.Active()
	if c == nil {
		return ErrNoCall
	}
	if err := c.SwitchMode(mode); err != nil {
		return s.fail(fmt.Errorf("mode: %w", err))
	}
	s.note("mode", "name", proto.ModeName(c.Mode()))
	s.emitState()
	return nil
}

func (s *Session) SwitchProfileName(name string) error {
	profile, ok := proto.LookupProfile(strings.ToLower(strings.TrimSpace(name)))
	if !ok {
		return s.fail(fmt.Errorf("profile: %w", ErrUnknownName))
	}
	return s.SwitchProfile(profile)
}

func (s *Session) SwitchModeName(name string) error {
	mode, ok := proto.LookupMode(strings.ToLower(strings.TrimSpace(name)))
	if !ok {
		return s.fail(fmt.Errorf("mode: %w", ErrUnknownName))
	}
	return s.SwitchMode(mode)
}

func (s *Session) PushPCM(pcm []int16) error {
	if s.host == nil {
		return ErrNoHost
	}
	before := s.host.Stats().CaptureDropped
	if err := s.host.Push(pcm); err != nil {
		return s.fail(fmt.Errorf("push pcm: %w", err))
	}
	if s.host.Stats().CaptureDropped > before {
		return s.fail(ErrPCMDropped)
	}
	return nil
}

func (s *Session) PushPCMBytes(raw []byte) error {
	if s.host == nil {
		return ErrNoHost
	}
	before := s.host.Stats().CaptureDropped
	if err := s.host.PushBytes(raw); err != nil {
		return s.fail(fmt.Errorf("push pcm: %w", err))
	}
	if s.host.Stats().CaptureDropped > before {
		return s.fail(ErrPCMDropped)
	}
	return nil
}

func (s *Session) PullPCM() ([]int16, bool) {
	if s.host == nil {
		return nil, false
	}
	return s.host.Pull()
}

func (s *Session) PullPCMBytes() ([]byte, bool) {
	if s.host == nil {
		return nil, false
	}
	return s.host.PullBytes()
}

func (s *Session) WaitPCM(ctx context.Context) ([]int16, error) {
	if s.host == nil {
		return nil, ErrNoHost
	}
	return s.host.WaitPlayback(ctx)
}

func (s *Session) Close() error {
	_ = s.Hangup()
	s.phone.Stop()
	s.note("close", "dest", call.FormatHash(s.phone.DestHash()))
	if s.owned && s.host != nil {
		if err := s.host.Close(); err != nil {
			return fmt.Errorf("close host: %w", err)
		}
	}
	return nil
}

func (s *Session) callEvents() call.Events {
	return call.Events{
		OnRinging: func(c *call.Call) {
			s.note("ringing", "remote", call.Fingerprint(c.RemoteIdentity()), "incoming", boolField(c.Incoming()))
			s.emitState()
			s.mutex.Lock()
			fn := s.events.OnRinging
			s.mutex.Unlock()
			if fn != nil {
				fn(c.RemoteIdentity(), c.Incoming())
			}
		},
		OnAnswered: func(*call.Call) {
			s.note("answered")
			s.emitState()
			s.mutex.Lock()
			fn := s.events.OnAnswered
			s.mutex.Unlock()
			if fn != nil {
				fn()
			}
		},
		OnBusy: func(*call.Call) {
			s.note("busy_remote")
			s.emitState()
			s.mutex.Lock()
			fn := s.events.OnBusy
			s.mutex.Unlock()
			if fn != nil {
				fn()
			}
		},
		OnRejected: func(*call.Call) {
			s.note("rejected")
			s.emitState()
			s.mutex.Lock()
			fn := s.events.OnRejected
			s.mutex.Unlock()
			if fn != nil {
				fn()
			}
		},
		OnEnded: func(c *call.Call, reason string) {
			s.mutex.Lock()
			s.reason = reason
			fn := s.events.OnEnded
			s.mutex.Unlock()
			if err := c.EndError(); err != nil {
				_ = s.fail(err)
			}
			s.note("ended", "reason", reason)
			s.emitState()
			if fn != nil {
				fn(reason)
			}
		},
		OnStats: func(_ *call.Call, m media.LinkMetrics) {
			s.mutex.Lock()
			fn := s.events.OnStats
			s.mutex.Unlock()
			if fn != nil {
				fn(m)
			}
		},
	}
}

func (s *Session) emitState() {
	info := s.Info()
	s.mutex.Lock()
	fn := s.events.OnState
	s.mutex.Unlock()
	if fn != nil {
		fn(info)
	}
}

func (s *Session) fail(err error) error {
	if err == nil {
		return nil
	}
	s.mutex.Lock()
	s.lastErr = err
	fn := s.events.OnError
	s.mutex.Unlock()
	s.note("error", "err", err.Error())
	if fn != nil {
		fn(err)
	}
	return err
}

func (s *Session) note(event string, kv ...string) {
	s.mutex.Lock()
	logFn := s.logFn
	s.mutex.Unlock()
	if logFn != nil {
		logFn(event, kv...)
	}
	if !debug.Enabled(debug.DebugInfo) {
		return
	}
	args := make([]any, 0, len(kv))
	for _, v := range kv {
		args = append(args, v)
	}
	debug.Log(debug.DebugInfo, "lxst.session "+event, args...)
}

func policyName(policy byte, allowed [][]byte, fn func([]byte) bool) string {
	if fn != nil {
		return "func"
	}
	if policy == phonebook.AllowNone {
		return "none"
	}
	if policy == 0 && len(allowed) > 0 {
		return "list"
	}
	if policy == phonebook.AllowAll || policy == 0 {
		return "all"
	}
	return "list"
}
