// SPDX-License-Identifier: LicenseRef-Reticulum
package session

import (
	"context"
	"crypto/sha256"
	"sync"
	"sync/atomic"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/resource"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv/media"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rnv/proto"
)

// Conn is an established RNV session after HELLO exchange (or while awaiting it).
type Conn struct {
	ep       *Endpoint
	lnk      *link.Link
	peerHash []byte
	outbound bool

	mu         sync.Mutex
	localCaps  proto.Caps
	peerCaps   proto.Caps
	handshaken bool
	closed     bool
	helloCh    chan struct{}
	helloOnce  sync.Once

	stream  *Stream
	videoJB *media.JitterBuffer
	audioJB *media.JitterBuffer
	avClock media.Clock

	pendingStillID []byte
	pendingClipID  []byte
	pendingClipCh  chan clipResult
	resourceBuf    []byte
	resourceExpect uint64

	stillTimes []time.Time
	clipTimes  []time.Time

	droppedBadMagic uint64
}

type clipResult struct {
	meta proto.ClipMeta
	data []byte
	err  error
}

func newConn(ep *Endpoint, lnk *link.Link, peerHash []byte, outbound bool) *Conn {
	return &Conn{
		ep:        ep,
		lnk:       lnk,
		peerHash:  cloneHash(peerHash),
		outbound:  outbound,
		localCaps: ep.cfg.Caps,
		helloCh:   make(chan struct{}),
		videoJB:   media.NewJitterBuffer(0, 0),
		audioJB:   media.NewJitterBuffer(0, 0),
	}
}

func (c *Conn) attachLink() {
	_ = c.lnk.SetResourceStrategy(link.AcceptApp)
	c.lnk.SetPacketCallback(func(data []byte, _ *packet.Packet) {
		c.onPacket(data)
	})
	c.lnk.SetResourceCallback(func(adv any) bool {
		return c.onResourceAdv(adv)
	})
	c.lnk.SetResourceConcludedCallback(func(res any) {
		c.onResourceDone(res)
	})
	c.lnk.SetLinkClosedCallback(func(*link.Link) {
		c.markClosed()
	})
}

// PeerHash returns a copy of the remote identity hash.
func (c *Conn) PeerHash() []byte { return cloneHash(c.peerHash) }

// LocalCaps returns local HELLO caps.
func (c *Conn) LocalCaps() proto.Caps {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.localCaps
}

// PeerCaps returns remote HELLO caps (zero until handshake).
func (c *Conn) PeerCaps() proto.Caps {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.peerCaps
}

// Handshaken reports whether peer HELLO was received.
func (c *Conn) Handshaken() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.handshaken
}

func (c *Conn) sendHello() error {
	env := proto.NewTyped(proto.TypeHello, c.localCaps.ToBody())
	return c.sendEnvelope(env)
}

func (c *Conn) waitHello(timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-c.helloCh:
		return nil
	case <-timer.C:
		return rnv.ErrHelloTimeout
	}
}

func (c *Conn) sendEnvelope(env *EnvelopeOr) error {
	if env == nil {
		return rnv.ErrNilArgument
	}
	raw, err := env.Pack()
	if err != nil {
		return err
	}
	return c.sendRaw(raw)
}

// EnvelopeOr avoids importing confusion — use proto.Envelope.
type EnvelopeOr = proto.Envelope

func (c *Conn) sendRaw(raw []byte) error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed || c.lnk == nil || !c.lnk.IsActive() {
		return rnv.ErrLinkInactive
	}
	return c.lnk.SendPacket(raw)
}

func (c *Conn) onPacket(data []byte) {
	if proto.IsMediaFrame(data) {
		fr, err := proto.SplitFrame(data)
		if err != nil {
			atomic.AddUint64(&c.droppedBadMagic, 1)
			return
		}
		c.handleFrame(fr)
		return
	}
	env, err := proto.UnpackEnvelope(data)
	if err != nil {
		return
	}
	c.dispatch(env)
}

func (c *Conn) dispatch(env *proto.Envelope) {
	if env.Type >= proto.TypePrivateMin {
		c.handlePrivate(env)
		return
	}
	switch env.Type {
	case proto.TypeHello:
		caps := proto.CapsFromBody(env.Body)
		c.mu.Lock()
		c.peerCaps = caps
		c.handshaken = true
		c.mu.Unlock()
		c.helloOnce.Do(func() { close(c.helloCh) })
		for id, payload := range env.Extensions {
			c.fireExt(id, payload)
		}
		for id, payload := range caps.Extensions {
			c.fireExt(id, payload)
		}
	case proto.TypeReject:
		body := proto.RejectFromBody(env.Body)
		c.noteReject(body)
		if h := c.ep.cfg.Handlers.OnReject; h != nil {
			h(c, body)
		}
	case proto.TypeStill:
		c.handleStillMeta(proto.StillMetaFromBody(env.Body))
	case proto.TypeClipOffer:
		c.handleClipOffer(proto.ClipMetaFromBody(env.Body))
	case proto.TypeClipAccept:
		c.signalClipAccept(proto.ClipMetaFromBody(env.Body))
	case proto.TypeClipDone:
		c.signalClipDone(proto.ClipMetaFromBody(env.Body))
	case proto.TypeStreamOffer:
		c.handleStreamOffer(proto.StreamOfferFromBody(env.Body))
	case proto.TypeStreamAccept:
		c.handleStreamAccept(proto.StreamOfferFromBody(env.Body))
	case proto.TypeStreamCtrl:
		if h := c.ep.cfg.Handlers.OnCtrl; h != nil {
			h(c, proto.CtrlFromBody(env.Body))
		}
	case proto.TypeBye:
		if h := c.ep.cfg.Handlers.OnBye; h != nil {
			h(c)
		}
		_ = c.Close()
	default:
		if c.localCaps.StrictExt || c.ep.cfg.StrictExtensions {
			_ = c.sendReject(proto.RejectUnknown, "unknown type")
		}
	}
}

func (c *Conn) handlePrivate(env *proto.Envelope) {
	if c.ep.cfg.StrictExtensions || c.localCaps.StrictExt {
		_ = c.sendReject(proto.RejectUnknown, "unknown private type")
		return
	}
	for id, payload := range env.Extensions {
		c.fireExt(id, payload)
	}
}

func (c *Conn) fireExt(id uint64, payload []byte) {
	if h := c.ep.cfg.Handlers.OnExtension; h != nil {
		h(c, id, append([]byte(nil), payload...))
	}
}

func (c *Conn) handleFrame(fr proto.Frame) {
	switch fr.Magic {
	case proto.MagicVideo:
		c.videoJB.PushOwned(fr.Seq, fr.Payload)
		c.avClock.NoteVideo(fr.Seq)
		if h := c.ep.cfg.Handlers.OnVideo; h != nil {
			h(c, fr)
		}
	case proto.MagicAudio:
		c.audioJB.PushOwned(fr.Seq, fr.Payload)
		c.avClock.NoteAudio(fr.Seq)
		if h := c.ep.cfg.Handlers.OnAudio; h != nil {
			h(c, fr)
		}
	}
}

func (c *Conn) sendReject(code int, reason string) error {
	env := proto.NewTyped(proto.TypeReject, proto.RejectBody{Code: code, Reason: reason}.ToBody())
	return c.sendEnvelope(env)
}

func (c *Conn) requireHandshake() error {
	c.mu.Lock()
	ok := c.handshaken
	c.mu.Unlock()
	if !ok {
		return rnv.ErrNotHandshaken
	}
	return nil
}

func (c *Conn) effectiveStillMax() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, b := c.localCaps.MaxStill, c.peerCaps.MaxStill
	if a == 0 {
		a = rnv.MaxStillBytes
	}
	if b == 0 {
		b = rnv.MaxStillBytes
	}
	if a < b {
		return a
	}
	return b
}

func (c *Conn) effectiveClipMax() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, b := c.localCaps.MaxClip, c.peerCaps.MaxClip
	if a == 0 {
		a = rnv.MaxClipBytes
	}
	if b == 0 {
		b = rnv.MaxClipBytes
	}
	if a < b {
		return a
	}
	return b
}

func (c *Conn) markClosed() {
	c.mu.Lock()
	c.closed = true
	if c.stream != nil {
		c.stream.markClosed()
	}
	c.mu.Unlock()
}

// Close sends BYE and tears down the link.
func (c *Conn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	_ = c.sendEnvelope(proto.NewTyped(proto.TypeBye, nil))
	if c.lnk != nil {
		c.lnk.Teardown()
	}
	return nil
}

func (c *Conn) onResourceAdv(adv any) bool {
	var size int64
	switch a := adv.(type) {
	case *resource.ResourceAdvertisement:
		if a != nil {
			size = a.DataSize
		}
	}
	max := int64(c.effectiveClipMax()) // #nosec G115
	if max > 0 && size > max {
		return false
	}
	if size > int64(rnv.MaxClipBytes) && !c.ep.cfg.DangerousRaiseLimits {
		return false
	}
	c.mu.Lock()
	c.resourceExpect = uint64(size) // #nosec G115
	c.mu.Unlock()
	return true
}

func (c *Conn) onResourceDone(res any) {
	data := extractResourceData(res)
	if len(data) == 0 {
		return
	}
	c.mu.Lock()
	stillID := cloneHash(c.pendingStillID)
	clipCh := c.pendingClipCh
	c.pendingStillID = nil
	c.resourceBuf = nil
	c.mu.Unlock()

	if len(stillID) > 0 {
		meta := proto.StillMeta{Transfer: proto.TransferResource, Size: uint64(len(data)), ID: stillID}
		if h := c.ep.cfg.Handlers.OnStill; h != nil {
			h(c, meta, data)
		}
		return
	}
	if clipCh != nil {
		select {
		case clipCh <- clipResult{data: data}:
		default:
		}
	}
}

func extractResourceData(res any) []byte {
	switch v := res.(type) {
	case []byte:
		return v
	case link.IncomingResource:
		return append([]byte(nil), v.Data...)
	default:
		return nil
	}
}

func (c *Conn) sendPayload(ctx context.Context, payload []byte, timeout time.Duration) error {
	if len(payload) <= proto.LinkPacketSoftMax {
		return c.sendRaw(payload)
	}
	res, err := resource.New(payload, true)
	if err != nil {
		return err
	}
	// SendResource blocks until the peer proves receipt or the link times out.
	done := make(chan error, 1)
	go func() {
		done <- c.lnk.SendResource(res)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			return err
		}
		return nil
	case <-timer.C:
		return rnv.ErrResourceTimeout
	case <-ctx.Done():
		return ctx.Err()
	}
}

func contentHash(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:8]
}

func (c *Conn) rateStillOK() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	cut := now.Add(-time.Minute)
	kept := c.stillTimes[:0]
	for _, t := range c.stillTimes {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	c.stillTimes = kept
	if len(c.stillTimes) >= c.ep.cfg.MaxStillsPerMinute {
		return false
	}
	c.stillTimes = append(c.stillTimes, now)
	return true
}

func (c *Conn) rateClipOK() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	cut := now.Add(-time.Minute)
	kept := c.clipTimes[:0]
	for _, t := range c.clipTimes {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	c.clipTimes = kept
	if len(c.clipTimes) >= c.ep.cfg.MaxClipsPerMinute {
		return false
	}
	c.clipTimes = append(c.clipTimes, now)
	return true
}

// DroppedBadMagic returns count of discarded non-frame/non-envelope packets misclassified.
func (c *Conn) DroppedBadMagic() uint64 {
	return atomic.LoadUint64(&c.droppedBadMagic)
}

// ensureActiveLXSTGuard is used before opening audio tracks.
func (c *Conn) ensureActiveLXSTGuard(tracks byte) error {
	if tracks&proto.TrackAudio == 0 {
		return nil
	}
	if c.ep.cfg.AllowParallelLXST {
		return nil
	}
	if c.ep.cfg.LXSTActive != nil && c.ep.cfg.LXSTActive(c.peerHash) {
		return rnv.ErrParallelLXST
	}
	return nil
}
