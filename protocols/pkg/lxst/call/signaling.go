// SPDX-License-Identifier: LicenseRef-Reticulum
package call

import (
	"context"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/proto"
)

func (c *Call) resendAvailableUntilIdentified() {
	for range availableResendCount {
		if c.sleepEnded(availableResendGap) {
			return
		}
		if !c.incoming.Load() {
			return
		}
		if c.state.Load() != int32(StateConnecting) {
			return
		}
		if c.status.Load() >= int32(proto.StatusRinging) {
			return
		}
		_ = c.sendSignals(proto.StatusAvailable)
	}
}

func (c *Call) onOutgoingEstablished(l *link.Link) {
	c.setLink(l)
	l.SetPacketCallback(c.onPacket)
	l.SetLinkClosedCallback(c.onLinkClosed)
	c.armTimeout(c.cfg.ConnectTime, func() bool {
		return c.status.Load() < int32(proto.StatusRinging)
	}, "connect timeout")
	go c.identifyIfStillConnecting()
}

func (c *Call) identifyOnce() {
	if c.incoming.Load() || c.cfg.Identity == nil {
		return
	}
	if c.status.Load() >= int32(proto.StatusRinging) {
		return
	}
	l := c.getLink()
	if l == nil || !l.IsActive() {
		return
	}
	if !c.identified.CompareAndSwap(false, true) {
		return
	}
	if err := l.Identify(c.cfg.Identity); err != nil {
		c.identified.Store(false)
		debug.Log(debug.DebugInfo, "call identify failed", "error", err)
	}
}

func (c *Call) identifyIfStillConnecting() {
	if c.sleepEnded(identifyDelay) {
		return
	}
	for range identifyRetryCount {
		if c.state.Load() != int32(StateConnecting) {
			return
		}
		if c.status.Load() >= int32(proto.StatusRinging) {
			return
		}
		c.identifyOnce()
		if c.identified.Load() {
			return
		}
		if c.sleepEnded(identifyRetryGap) {
			return
		}
	}
}

func (c *Call) onIdentified(_ *link.Link, id *identity.Identity) {
	if !c.incoming.Load() {
		return
	}
	if !c.callerAllowed(id) {
		c.rejectIncoming(ErrBlocked)
		return
	}
	if c.limiter != nil && id != nil && !c.limiter.Allow(id.Hash()) {
		c.rejectIncoming(ErrRateLimited)
		return
	}
	if !c.casState(StateConnecting, StateRinging) {
		return
	}
	c.status.Store(int32(proto.StatusRinging))
	_ = c.sendSignals(proto.StatusRinging)
	if c.events.OnRinging != nil {
		c.events.OnRinging(c)
	}
	c.startRingtone()
	if c.cfg.AutoAnswer > 0 {
		go c.autoAnswer()
	}
	c.armTimeout(c.cfg.RingTime, func() bool {
		return c.status.Load() < int32(proto.StatusEstablished)
	}, "ring timeout")
}

func (c *Call) autoAnswer() {
	if c.sleepEnded(c.cfg.AutoAnswer) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.ConnectTime)
	defer cancel()
	_ = c.Answer(ctx)
}

func (c *Call) onLinkClosed(_ *link.Link) {
	if !c.incoming.Load() && c.state.Load() == int32(StateRinging) && c.events.OnRejected != nil {
		c.events.OnRejected(c)
	}
	c.end("link closed")
}

func (c *Call) onPacket(data []byte, _ *packet.Packet) {
	if len(data) > proto.MaxUnpackBytes {
		return
	}
	msg, err := proto.Unpack(data)
	if err != nil {
		debug.Log(debug.DebugInfo, "lxst unpack failed", "error", err, "bytes", len(data))
		return
	}
	if len(msg.Signals) > 0 {
		c.handleSignals(msg.Signals)
	}
	for _, frame := range msg.Frames {
		c.handleFrame(frame)
	}
}

func (c *Call) handleSignals(signals []int) {
	c.sigMu.Lock()
	defer c.sigMu.Unlock()
	for _, signal := range signals {
		c.handleSignal(signal)
	}
}

func (c *Call) handleSignal(signal int) {
	if c.incoming.Load() && !c.answered.Load() && signal < proto.PreferredMode {
		return
	}
	switch {
	case signal == proto.StatusBusy:
		c.signalBusy()
	case signal == proto.StatusRejected:
		c.signalRejected()
	case signal == proto.StatusAvailable:
		c.status.Store(int32(signal))
		c.identifyOnce()
	case signal == proto.StatusRinging:
		c.signalRinging()
	case signal == proto.StatusConnecting:
		c.signalConnecting()
	case signal == proto.StatusEstablished:
		c.signalEstablished()
	case proto.IsPreferredProfile(signal):
		c.applyPreferredProfile(signal)
	case proto.IsPreferredMode(signal):
		c.applyPreferredMode(signal)
	}
}

func (c *Call) signalBusy() {
	if c.events.OnBusy != nil {
		c.events.OnBusy(c)
	}
	c.startBusyTone()
	c.end("busy")
}

func (c *Call) signalRejected() {
	if c.events.OnRejected != nil {
		c.events.OnRejected(c)
	}
	c.startBusyTone()
	c.end("rejected")
}

func (c *Call) signalRinging() {
	c.status.Store(int32(proto.StatusRinging))
	if c.casState(StateConnecting, StateRinging) || c.state.Load() == int32(StateRinging) {
		c.applyProfile(int(c.profile.Load()))
		_ = c.sendSignals(
			proto.SignalPreferredProfile(int(c.profile.Load())),
			proto.SignalPreferredMode(int(c.callMode.Load())),
		)
		if c.events.OnRinging != nil {
			c.events.OnRinging(c)
		}
		if !c.incoming.Load() {
			c.startDialTone()
		}
	}
}

func (c *Call) signalConnecting() {
	c.status.Store(int32(proto.StatusConnecting))
	if !c.incoming.Load() {
		_ = c.openPipelines()
	}
}

func (c *Call) signalEstablished() {
	if !c.incoming.Load() {
		_ = c.openPipelines()
		became := c.casState(StateRinging, StateActive) || c.casState(StateConnecting, StateActive)
		if became {
			c.status.Store(int32(proto.StatusEstablished))
		}
		c.armTX()
		_ = c.startPipelines()
		if became && c.events.OnAnswered != nil {
			c.events.OnAnswered(c)
		}
		return
	}
	c.status.Store(int32(proto.StatusEstablished))
}

func (c *Call) rejectIncoming(err error) {
	c.mutex.Lock()
	c.endErr = err
	c.mutex.Unlock()
	if c.events.OnBusy != nil {
		c.events.OnBusy(c)
	}
	_ = c.sendSignals(proto.StatusBusy)
	c.end(err.Error())
}

func (c *Call) applyPreferredProfile(signal int) {
	profile := proto.ProfileFromSignal(signal)
	if profile == 0 {
		profile = proto.DefaultProfile
	}
	if !proto.KnownProfile(profile) {
		debug.Log(debug.DebugError, "lxst ignore unknown remote profile", profile)
		return
	}
	if c.state.Load() == int32(StateActive) {
		_ = c.switchProfile(profile)
		return
	}
	c.applyProfile(profile)
}

func (c *Call) applyPreferredMode(signal int) {
	if c.modeFollowOff.Load() {
		return
	}
	mode := proto.ModeFromSignal(signal)
	if mode != proto.ModeFullDuplex && mode != proto.ModeHalfDuplex {
		return
	}
	c.callMode.Store(int32(mode))
	if c.state.Load() == int32(StateActive) {
		c.setSquelchState(mode == proto.ModeHalfDuplex)
	} else {
		c.squelched.Store(mode == proto.ModeHalfDuplex)
	}
}

func (c *Call) handleFrame(frame []byte) {
	if c.incoming.Load() && !c.answered.Load() {
		return
	}
	st := c.state.Load()
	if st == int32(StateIdle) || st == int32(StateEnded) {
		return
	}
	if len(frame) < 1 {
		return
	}
	seq := uint16(c.frameSeq.Add(1)) // #nosec G115 -- 16-bit media sequence wraps
	c.jitter.PushOwned(seq, 0, frame)
	c.recvCount.Add(1)
	c.wakeRecv()
}

func (c *Call) sendSignals(signals ...int) error {
	l := c.getLink()
	if l == nil || !l.IsActive() {
		return ErrNotActive
	}
	for _, s := range signals {
		if proto.IsAutoStatus(s) && s >= int(c.status.Load()) {
			c.status.Store(clampInt32(s))
		}
	}
	payload, err := proto.PackSignalling(signals)
	if err != nil {
		return err
	}
	return l.SendPacket(payload)
}

func (c *Call) applyProfile(profile int) {
	if profile == 0 {
		profile = proto.DefaultProfile
	}
	c.profile.Store(clampInt32(profile))
	params := proto.ProfileParams(profile)
	c.mutex.Lock()
	c.params = params
	c.mutex.Unlock()
	c.jitter.SetTargetMs(params.FrameMs * params.BufferN)
}

func (c *Call) switchProfile(profile int) error {
	if c.state.Load() != int32(StateActive) {
		c.applyProfile(profile)
		return nil
	}
	params := proto.ProfileParams(profile)
	enc, err := newProfileEncoder(params)
	if err != nil {
		return err
	}
	dec, err := newProfileDecoder(params)
	if err != nil {
		_ = enc.Close()
		return err
	}
	c.applyProfile(profile)
	c.mutex.Lock()
	oldEnc := c.encoder
	oldDec := c.decoder
	c.encoder = enc
	c.decoder = dec
	c.params = params
	c.mutex.Unlock()
	if oldEnc != nil {
		_ = oldEnc.Close()
	}
	if oldDec != nil {
		_ = oldDec.Close()
	}
	return nil
}
