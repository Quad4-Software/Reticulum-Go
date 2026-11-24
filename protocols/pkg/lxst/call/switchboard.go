// SPDX-License-Identifier: LicenseRef-Reticulum
package call

import (
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/proto"
)

const (
	busyLinkTries     = 8
	busyLinkWait      = 2 * time.Millisecond
	busyTeardownDelay = 8 * time.Millisecond
)

// Switchboard accepts incoming LXST telephony calls on a destination.
type Switchboard struct {
	transport *transport.Transport
	cfg       Config
	handler   func(*Call)
	limiter   *RateLimiter

	mutex      sync.Mutex
	active     *Call
	forcedBusy bool
}

func NewSwitchboard(t *transport.Transport, cfg Config, handler func(*Call)) *Switchboard {
	return &Switchboard{
		transport: t,
		cfg:       cfg,
		handler:   handler,
	}
}

func (s *Switchboard) SetRateLimiter(l *RateLimiter) {
	s.mutex.Lock()
	s.limiter = l
	s.mutex.Unlock()
}

func (s *Switchboard) SetBusy(busy bool) {
	s.mutex.Lock()
	s.forcedBusy = busy
	s.mutex.Unlock()
}

func (s *Switchboard) setAudio(cfg Config) {
	s.mutex.Lock()
	s.cfg.RingtonePath = cfg.RingtonePath
	s.cfg.RingtoneGain = cfg.RingtoneGain
	s.cfg.Speaker = cfg.Speaker
	s.cfg.Microphone = cfg.Microphone
	s.cfg.Ringer = cfg.Ringer
	s.cfg.Device = cfg.Device
	s.cfg.UseAudio = cfg.UseAudio
	s.mutex.Unlock()
}

func (s *Switchboard) SetAccess(policy byte, allowed, blocked [][]byte, fn func([]byte) bool) {
	s.mutex.Lock()
	s.cfg.AllowPolicy = policy
	s.cfg.Allowed = allowed
	s.cfg.Blocked = blocked
	s.cfg.AllowFunc = fn
	s.mutex.Unlock()
}

func (s *Switchboard) Bind(dest *destination.Destination) {
	if dest == nil {
		return
	}
	dest.AcceptsLinks(true)
	if s.cfg.Identity == nil {
		s.cfg.Identity = dest.GetIdentity()
	}
	dest.SetLinkEstablishedCallback(func(l any) {
		lnk, ok := l.(*link.Link)
		if !ok {
			return
		}
		s.mutex.Lock()
		cfg := s.cfg
		limiter := s.limiter
		busy := s.busyLocked()
		s.mutex.Unlock()
		if busy {
			s.rejectBusy(lnk)
			return
		}
		incoming := NewCall(s.transport, cfg)
		if !s.Occupy(incoming) {
			s.rejectBusy(lnk)
			return
		}
		incoming.SetRateCheck(limiter)
		if err := incoming.ServeIncoming(lnk); err != nil {
			s.mutex.Lock()
			if s.active == incoming {
				s.active = nil
			}
			s.mutex.Unlock()
			lnk.Teardown()
			return
		}
		if s.handler != nil {
			s.handler(incoming)
		}
	})
}

func (*Switchboard) rejectBusy(lnk *link.Link) {
	payload, err := proto.PackSignalling([]int{proto.StatusBusy})
	if err == nil {
		for i := 0; i < busyLinkTries && !lnk.IsActive(); i++ {
			time.Sleep(busyLinkWait)
		}
		_ = lnk.SendPacket(payload)
	}
	time.Sleep(busyTeardownDelay)
	lnk.Teardown()
}

func (s *Switchboard) busyLocked() bool {
	if s.forcedBusy {
		return true
	}
	if s.active == nil {
		return false
	}
	st := s.active.State()
	if st == StateIdle || st == StateEnded {
		s.active = nil
		return false
	}
	return true
}

func (s *Switchboard) Occupy(c *Call) bool {
	if c == nil {
		return false
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.busyLocked() {
		return false
	}
	prev := c.events.OnEnded
	c.events.OnEnded = func(ended *Call, reason string) {
		s.Release(ended)
		if prev != nil {
			prev(ended, reason)
		}
	}
	s.active = c
	return true
}

func (s *Switchboard) Release(c *Call) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.active == c {
		s.active = nil
	}
}

func (s *Switchboard) Active() *Call {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.active != nil && (s.active.State() == StateEnded || s.active.State() == StateIdle) {
		s.active = nil
	}
	return s.active
}
