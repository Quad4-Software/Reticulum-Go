// SPDX-License-Identifier: LicenseRef-Reticulum

// Package session is a hash-addressed LXMF messenger API.
//
// Peers are identity and lxmf.delivery destination hashes, not hosts or URLs.
// The app supplies a Reticulum transport. This package does not open sockets
// onto the mesh and does not terminate LXMF on a server.
//
// Announce is explicit. A received message is not trust. Show the sender
// fingerprint from OnMessage or Info.LastFrom. SignatureValidated is the
// cryptographic proof. DropUnverified and RequireStamp are local gates.
//
// GoSend and GoSendHash return immediately. Failures go to OnError and
// LastError. Store-and-forward through a propagation node is explicit via
// SetPropagationHash and SendPropagated. This package does not pick a node.
//
// lxm:// paper URIs are out of band. They are not a Reticulum hop.
//
// Info, Events, and Config.Log are local diagnostics. They do not change
// the LXMF wire format.
package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

const (
	defaultRecall           = 10 * time.Second
	defaultAnnounceInterval = 3 * time.Hour
	recallPollInterval      = 100 * time.Millisecond
)

type LogFunc func(event string, kv ...string)

type Events struct {
	OnMessage    func(*lxmf.LXMessage)
	OnUnverified func(*lxmf.LXMessage)
	OnDropped    func(*lxmf.LXMessage, error)
	OnState      func(Info)
	OnError      func(error)
}

type Config struct {
	Transport        *transport.Transport
	Identity         *identity.Identity
	Destination      *destination.Destination
	DisplayName      string
	StampCost        int
	RecallTimeout    time.Duration
	AnnounceInterval time.Duration
	Allowed          [][]byte
	Blocked          [][]byte
	AllowFunc        func([]byte) bool
	AllowNone        bool
	DropUnverified   bool
	RequireStamp     bool
	RatchetPath      string
	Events           Events
	Log              LogFunc
	NoRateLimit      bool
	RateInterval     time.Duration
	RateBurst        int
	PropagationHash  []byte
}

type Session struct {
	cfg       Config
	messenger *lxmf.Messenger
	recall    time.Duration
	logFn     LogFunc
	limiter   *rateLimiter

	mutex          sync.Mutex
	events         Events
	lastErr        error
	announced      bool
	sent           uint64
	recv           uint64
	dropped        uint64
	lastFrom       []byte
	lastTitle      string
	allowed        [][]byte
	blocked        [][]byte
	allowFunc      func([]byte) bool
	allowNone      bool
	propHash       []byte
	announceCancel context.CancelFunc
}

func Open(cfg Config) (*Session, error) {
	if cfg.Transport == nil {
		return nil, ErrNoTransport
	}
	if cfg.Identity == nil {
		return nil, ErrNoIdentity
	}
	if cfg.RequireStamp && cfg.StampCost <= 0 {
		return nil, ErrRequireStampCost
	}
	recall := cfg.RecallTimeout
	if recall <= 0 {
		recall = defaultRecall
	}
	s := &Session{
		cfg:       cfg,
		recall:    recall,
		events:    cfg.Events,
		logFn:     cfg.Log,
		allowed:   cloneHashes(cfg.Allowed),
		blocked:   cloneHashes(cfg.Blocked),
		allowFunc: cfg.AllowFunc,
		allowNone: cfg.AllowNone,
		propHash:  append([]byte(nil), cfg.PropagationHash...),
	}
	if !cfg.NoRateLimit {
		s.limiter = newRateLimiter(cfg.RateInterval, cfg.RateBurst)
	}
	dest := cfg.Destination
	if dest == nil {
		var err error
		dest, err = lxmf.NewDeliveryDestination(cfg.Identity, cfg.Transport)
		if err != nil {
			return nil, fmt.Errorf("delivery destination: %w", err)
		}
	}
	appData, err := lxmf.EncodeAnnounceAppDataV5(cfg.DisplayName, int64(max(cfg.StampCost, 0)))
	if err != nil {
		return nil, fmt.Errorf("announce app data: %w", err)
	}
	dest.SetDefaultAppData(appData)
	s.messenger = lxmf.NewMessenger(cfg.Transport, dest)
	if cfg.RatchetPath != "" {
		s.messenger.EnableRatchets(cfg.RatchetPath)
	}
	s.messenger.SetMessageHandler(s.onInbound)
	s.messenger.SetReceiveError(func(err error) {
		_ = s.fail(err)
	})
	rate := "on"
	if cfg.NoRateLimit {
		rate = "off"
	}
	prop := "none"
	if len(s.propHash) == lxmf.DestinationLength {
		prop = FormatHash(s.propHash)
	}
	s.note("open",
		"dest", FormatHash(s.messenger.DestinationHash()),
		"aspect", lxmf.AppName+"."+lxmf.AspectDelivery,
		"allow", s.policyName(),
		"announce", "manual",
		"stamp", fmt.Sprintf("%d", cfg.StampCost),
		"rate_limit", rate,
		"prop", prop,
	)
	return s, nil
}

// Messenger returns the underlying LXMF messenger for protocol-level work.
func (s *Session) Messenger() *lxmf.Messenger {
	return s.messenger
}

func (s *Session) DestHash() []byte {
	return s.messenger.DestinationHash()
}

func (s *Session) Announce() error {
	err := s.messenger.Destination().Announce(false, nil, nil)
	if err != nil {
		return s.fail(fmt.Errorf("announce: %w", err))
	}
	s.mutex.Lock()
	s.announced = true
	s.mutex.Unlock()
	s.note("announce", "dest", FormatHash(s.messenger.DestinationHash()))
	return nil
}

func (s *Session) StartAnnounce(ctx context.Context) {
	interval := s.cfg.AnnounceInterval
	if interval <= 0 {
		interval = defaultAnnounceInterval
	}
	s.mutex.Lock()
	if s.announceCancel != nil {
		s.announceCancel()
	}
	ctx, cancel := context.WithCancel(ctx)
	s.announceCancel = cancel
	s.mutex.Unlock()
	_ = s.Announce()
	s.note("announce_loop", "dest", FormatHash(s.messenger.DestinationHash()))
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.Announce()
			}
		}
	}()
}

func (s *Session) SetAllowed(hashes [][]byte, fn func([]byte) bool) {
	s.mutex.Lock()
	s.allowed = cloneHashes(hashes)
	s.allowFunc = fn
	s.allowNone = false
	s.mutex.Unlock()
}

func (s *Session) SetAllowNone(none bool) {
	s.mutex.Lock()
	s.allowNone = none
	s.mutex.Unlock()
}

func (s *Session) SetBlocked(hashes [][]byte) {
	s.mutex.Lock()
	s.blocked = cloneHashes(hashes)
	s.mutex.Unlock()
}

func (s *Session) SetOnMessage(fn func(*lxmf.LXMessage)) {
	s.mutex.Lock()
	s.events.OnMessage = fn
	s.mutex.Unlock()
}

func (s *Session) SetOnDropped(fn func(*lxmf.LXMessage, error)) {
	s.mutex.Lock()
	s.events.OnDropped = fn
	s.mutex.Unlock()
}

func (s *Session) SetPropagationHash(hash []byte) error {
	if len(hash) == 0 {
		s.mutex.Lock()
		s.propHash = nil
		s.mutex.Unlock()
		s.note("prop", "hash", "none")
		return nil
	}
	if len(hash) != lxmf.DestinationLength {
		return s.fail(fmt.Errorf("propagation: %w", ErrInvalidHash))
	}
	s.mutex.Lock()
	s.propHash = append([]byte(nil), hash...)
	s.mutex.Unlock()
	s.note("prop", "hash", FormatHash(hash))
	return nil
}

func (s *Session) Send(ctx context.Context, remote *identity.Identity, title, content string) (*lxmf.LXMessage, error) {
	if remote == nil {
		return nil, s.fail(fmt.Errorf("send: %w", ErrNoIdentity))
	}
	dest, err := lxmf.DeliveryHash(remote)
	if err != nil {
		return nil, s.fail(fmt.Errorf("send: %w", err))
	}
	identity.Remember(nil, dest, remote.GetPublicKey(), nil)
	return s.sendDest(ctx, dest, title, content, false)
}

func (s *Session) SendHash(ctx context.Context, destHex, title, content string) (*lxmf.LXMessage, error) {
	dest, err := s.resolveDest(ctx, destHex)
	if err != nil {
		return nil, err
	}
	return s.sendDest(ctx, dest, title, content, false)
}

func (s *Session) SendDirect(ctx context.Context, destHex, title, content string) (*lxmf.LXMessage, error) {
	dest, err := s.resolveDest(ctx, destHex)
	if err != nil {
		return nil, err
	}
	return s.sendDest(ctx, dest, title, content, true)
}

func (s *Session) SendPropagated(ctx context.Context, destHex, title, content string) (*lxmf.LXMessage, error) {
	dest, err := s.resolveDest(ctx, destHex)
	if err != nil {
		return nil, err
	}
	s.mutex.Lock()
	prop := append([]byte(nil), s.propHash...)
	s.mutex.Unlock()
	if len(prop) != lxmf.DestinationLength {
		return nil, s.fail(fmt.Errorf("send: %w", ErrNoPropagation))
	}
	msg, err := s.messenger.Compose(dest, title, content, nil)
	if err != nil {
		return nil, s.fail(fmt.Errorf("send: %w", err))
	}
	if s.cfg.StampCost > 0 {
		if err := s.messenger.SendStampedPropagated(msg, prop, s.cfg.StampCost, lxmf.PropagationStampCostMin); err != nil {
			return nil, s.fail(fmt.Errorf("send: %w", err))
		}
	} else if err := s.messenger.SendPropagated(msg, prop, lxmf.PropagationStampCostMin); err != nil {
		return nil, s.fail(fmt.Errorf("send: %w", err))
	}
	s.mutex.Lock()
	s.sent++
	s.mutex.Unlock()
	s.note("sent", "remote", FormatHash(dest), "method", "propagated", "hash", msg.FormatHash())
	s.emitState()
	return msg, nil
}

// GoSend starts Send without blocking. Failures go to OnError and LastError.
func (s *Session) GoSend(ctx context.Context, remote *identity.Identity, title, content string) {
	go func() {
		_, _ = s.Send(ctx, remote, title, content)
	}()
}

func (s *Session) GoSendHash(ctx context.Context, destHex, title, content string) {
	go func() {
		_, _ = s.SendHash(ctx, destHex, title, content)
	}()
}

func (s *Session) sendDest(ctx context.Context, dest []byte, title, content string, direct bool) (*lxmf.LXMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, s.fail(fmt.Errorf("send: %w", err))
	}
	msg, err := s.messenger.Compose(dest, title, content, nil)
	if err != nil {
		return nil, s.fail(fmt.Errorf("send: %w", err))
	}
	method := "opportunistic"
	if direct {
		method = "direct"
		if s.cfg.StampCost > 0 {
			err = s.messenger.SendStampedDirect(msg, s.cfg.StampCost)
		} else {
			err = s.messenger.SendDirect(msg)
		}
	} else if s.cfg.StampCost > 0 {
		err = s.messenger.SendStampedContext(ctx, msg, s.cfg.StampCost)
	} else {
		err = s.messenger.Send(msg)
	}
	if err != nil {
		return nil, s.fail(fmt.Errorf("send: %w", err))
	}
	s.mutex.Lock()
	s.sent++
	s.mutex.Unlock()
	s.note("sent", "remote", FormatHash(dest), "method", method, "hash", msg.FormatHash())
	s.emitState()
	return msg, nil
}

func (s *Session) resolveDest(ctx context.Context, destHex string) ([]byte, error) {
	raw, err := ParseHash(destHex)
	if err != nil {
		return nil, s.fail(fmt.Errorf("send: %w", err))
	}
	candidates := [][]byte{raw}
	if derived := lxmf.DestHash(raw); len(derived) == lxmf.DestinationLength && !bytes.Equal(derived, raw) {
		candidates = append(candidates, derived)
	}
	s.note("recall", "hash", FormatHash(raw))
	timeout := s.recall
	if deadline, ok := ctx.Deadline(); ok {
		remain := time.Until(deadline)
		if remain < timeout {
			timeout = remain
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, s.fail(fmt.Errorf("send: %w", err))
	}
	remote, err := waitRecall(ctx, s.cfg.Transport, candidates, timeout)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, s.fail(fmt.Errorf("send: %w", err))
		}
		return nil, s.fail(fmt.Errorf("%w: %s", ErrRecall, FormatHash(raw)))
	}
	dest, err := lxmf.DeliveryHash(remote)
	if err != nil {
		return nil, s.fail(fmt.Errorf("send: %w", err))
	}
	if !bytes.Equal(raw, remote.Hash()) && !bytes.Equal(raw, dest) {
		return nil, s.fail(fmt.Errorf("send: %w", ErrFingerprint))
	}
	identity.Remember(nil, dest, remote.GetPublicKey(), nil)
	return dest, nil
}

func (s *Session) Close() error {
	s.mutex.Lock()
	cancel := s.announceCancel
	s.announceCancel = nil
	s.mutex.Unlock()
	if cancel != nil {
		cancel()
	}
	s.messenger.SetMessageHandler(nil)
	s.note("close", "dest", FormatHash(s.messenger.DestinationHash()))
	return nil
}

func (s *Session) onInbound(msg *lxmf.LXMessage, _ common.NetworkInterface) {
	if msg == nil {
		return
	}
	src := append([]byte(nil), msg.SourceHash...)
	if err := s.admit(msg); err != nil {
		s.mutex.Lock()
		s.dropped++
		fn := s.events.OnDropped
		s.mutex.Unlock()
		s.note("drop", "from", FormatHash(src), "err", err.Error())
		if fn != nil {
			fn(msg, err)
		}
		_ = s.fail(err)
		s.emitState()
		return
	}
	if !msg.SignatureValidated {
		s.note("unverified", "from", FormatHash(src))
		s.mutex.Lock()
		unv := s.events.OnUnverified
		s.mutex.Unlock()
		if unv != nil {
			unv(msg)
		}
		if s.cfg.DropUnverified {
			s.mutex.Lock()
			s.dropped++
			dropFn := s.events.OnDropped
			s.mutex.Unlock()
			if dropFn != nil {
				dropFn(msg, ErrUnverified)
			}
			_ = s.fail(ErrUnverified)
			s.emitState()
			return
		}
	}
	s.mutex.Lock()
	s.recv++
	s.lastFrom = src
	s.lastTitle = msg.TitleString()
	fn := s.events.OnMessage
	s.mutex.Unlock()
	s.note("recv", "from", FormatHash(src), "title", msg.TitleString(), "signed", boolField(msg.SignatureValidated))
	s.emitState()
	if fn != nil {
		fn(msg)
	}
}

func (s *Session) admit(msg *lxmf.LXMessage) error {
	src := msg.SourceHash
	s.mutex.Lock()
	blocked := s.blocked
	allowed := s.allowed
	fn := s.allowFunc
	none := s.allowNone
	s.mutex.Unlock()
	for _, b := range blocked {
		if bytes.Equal(b, src) {
			return ErrNotAllowed
		}
	}
	if none {
		return ErrNotAllowed
	}
	if fn != nil && !fn(src) {
		return ErrNotAllowed
	}
	if len(allowed) > 0 {
		ok := false
		for _, a := range allowed {
			if bytes.Equal(a, src) {
				ok = true
				break
			}
		}
		if !ok {
			return ErrNotAllowed
		}
	}
	if !s.limiter.Allow(src) {
		return ErrRateLimited
	}
	if s.cfg.RequireStamp {
		ok, err := msg.ValidateStamp(s.cfg.StampCost, nil)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrStamp, err)
		}
		if !ok {
			return ErrStamp
		}
	}
	return nil
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
	debug.Log(debug.DebugInfo, "lxmf.session "+event, args...)
}

func (s *Session) policyName() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.policyNameLocked()
}

func (s *Session) policyNameLocked() string {
	if s.allowFunc != nil {
		return "func"
	}
	if s.allowNone {
		return "none"
	}
	if len(s.allowed) > 0 {
		return "list"
	}
	return "all"
}

func cloneHashes(in [][]byte) [][]byte {
	if len(in) == 0 {
		return nil
	}
	out := make([][]byte, len(in))
	for i, h := range in {
		out[i] = append([]byte(nil), h...)
	}
	return out
}

type pathTransport interface {
	HasPath([]byte) bool
	RequestPath([]byte, string, []byte, bool) error
}

func waitRecall(ctx context.Context, t pathTransport, hashes [][]byte, timeout time.Duration) (*identity.Identity, error) {
	if t == nil {
		return nil, fmt.Errorf("missing transport")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, h := range hashes {
		if len(h) == 0 {
			continue
		}
		if !t.HasPath(h) {
			_ = t.RequestPath(h, "", nil, false)
		}
	}
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	timer := time.NewTimer(recallPollInterval)
	defer timer.Stop()
	for {
		for _, h := range hashes {
			if len(h) == 0 {
				continue
			}
			if remote, err := identity.Recall(h); err == nil {
				return remote, nil
			}
		}
		if !time.Now().Before(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("could not recall identity")
		case <-timer.C:
			timer.Reset(recallPollInterval)
		}
	}
	return nil, fmt.Errorf("could not recall identity")
}
