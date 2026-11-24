// SPDX-License-Identifier: LicenseRef-Reticulum
package call

import (
	"context"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/audio/io"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/proto"
)

const (
	DefaultAnnounceInterval = 3 * time.Hour
	MinAnnounceInterval     = 5 * time.Minute
)

// Phone owns a telephony destination, switchboard, and announce loop.
type Phone struct {
	cfg       Config
	transport *transport.Transport
	dest      *destination.Destination
	board     *Switchboard

	mutex   sync.Mutex
	cancel  context.CancelFunc
	lastAnn time.Time
}

func NewPhone(t *transport.Transport, dest *destination.Destination, cfg Config) *Phone {
	cfg = cfg.withDefaults()
	p := &Phone{cfg: cfg, transport: t, dest: dest}
	p.board = NewSwitchboard(t, cfg, nil)
	p.board.Bind(dest)
	return p
}

func (p *Phone) Destination() *destination.Destination {
	return p.dest
}

func (p *Phone) Switchboard() *Switchboard {
	return p.board
}

func (p *Phone) SetAllowed(policy byte, hashes [][]byte, fn func([]byte) bool) {
	p.mutex.Lock()
	p.cfg.AllowPolicy = policy
	p.cfg.Allowed = hashes
	p.cfg.AllowFunc = fn
	blocked := p.cfg.Blocked
	p.mutex.Unlock()
	p.board.SetAccess(policy, hashes, blocked, fn)
}

func (p *Phone) SetBusy(busy bool) {
	p.board.SetBusy(busy)
}

func (p *Phone) SetRingtone(path string, gainDB float64) {
	p.mutex.Lock()
	p.cfg.RingtonePath = path
	p.cfg.RingtoneGain = gainDB
	cfg := p.cfg
	p.mutex.Unlock()
	p.board.setAudio(cfg)
}

func (p *Phone) SetSpeaker(name string) {
	p.mutex.Lock()
	p.cfg.Speaker = name
	cfg := p.cfg
	p.mutex.Unlock()
	p.board.setAudio(cfg)
}

func (p *Phone) SetMicrophone(name string) {
	p.mutex.Lock()
	p.cfg.Microphone = name
	cfg := p.cfg
	p.mutex.Unlock()
	p.board.setAudio(cfg)
}

func (p *Phone) SetRinger(name string) {
	p.mutex.Lock()
	p.cfg.Ringer = name
	cfg := p.cfg
	p.mutex.Unlock()
	p.board.setAudio(cfg)
}

func (p *Phone) SetDevice(dev io.Device) {
	p.mutex.Lock()
	p.cfg.Device = dev
	if dev != nil {
		p.cfg.UseAudio = true
	}
	cfg := p.cfg
	p.mutex.Unlock()
	p.board.setAudio(cfg)
}

func (p *Phone) SetMode(mode int) {
	if mode == 0 {
		mode = proto.DefaultMode
	}
	p.mutex.Lock()
	p.cfg.Mode = mode
	p.mutex.Unlock()
}

func (p *Phone) SetBlocked(hashes [][]byte) {
	p.mutex.Lock()
	p.cfg.Blocked = hashes
	policy := p.cfg.AllowPolicy
	allowed := p.cfg.Allowed
	fn := p.cfg.AllowFunc
	p.mutex.Unlock()
	p.board.SetAccess(policy, allowed, hashes, fn)
}

func (p *Phone) Announce() error {
	p.mutex.Lock()
	gap := MinAnnounceInterval
	if time.Since(p.lastAnn) < gap && !p.lastAnn.IsZero() {
		p.mutex.Unlock()
		return nil
	}
	p.lastAnn = time.Now()
	p.mutex.Unlock()
	if p.dest == nil {
		return nil
	}
	return p.dest.Announce(false, nil, nil)
}

func (p *Phone) StartAnnounce(ctx context.Context) {
	p.mutex.Lock()
	if p.cancel != nil {
		p.mutex.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	interval := p.cfg.AnnounceInterval
	p.mutex.Unlock()
	_ = p.Announce()
	go func() {
		tick := time.NewTicker(interval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				_ = p.Announce()
			}
		}
	}()
}

func (p *Phone) Stop() {
	p.mutex.Lock()
	cancel := p.cancel
	p.cancel = nil
	p.mutex.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (p *Phone) Dial(ctx context.Context, remote *identity.Identity) (*Call, error) {
	p.mutex.Lock()
	cfg := p.cfg
	p.mutex.Unlock()
	c := NewCall(p.transport, cfg)
	if !p.board.Occupy(c) {
		return nil, ErrAlreadyCall
	}
	if err := c.Dial(ctx, remote); err != nil {
		_ = c.Hangup("dial failed")
		p.board.Release(c)
		return c, err
	}
	return c, nil
}

func (p *Phone) DestHash() []byte {
	if p.dest == nil {
		return nil
	}
	return p.dest.GetHash()
}

func (*Phone) Aspect() string {
	return proto.AppName + "." + proto.AspectName
}
