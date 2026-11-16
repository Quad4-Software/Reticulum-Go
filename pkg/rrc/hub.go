// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

// HubConfig configures hub identity metadata and limits.
type HubConfig struct {
	Name     string
	Version  string
	Limits   HubLimits
	Handlers HubHandlers
	// IncludeMemberList puts an advisory member list in JOINED/PARTED bodies.
	IncludeMemberList bool
	// EnableResourceTransfer accepts TypeResourceEnvelope metadata (rrcd extension).
	EnableResourceTransfer bool
	// MaxResourceBytes is the advertised resource payload size limit when transfer is enabled.
	MaxResourceBytes uint64
	// Capabilities overrides WELCOME capability flags. Nil uses DefaultHubCapabilities.
	Capabilities map[uint64]any
	// Policy is optional daemon policy. Nil keeps open-room library defaults.
	Policy HubPolicy
	// OnInboundBytes is called for each inbound packet size in bytes.
	OnInboundBytes func(n int)
	// OnBadPacket is called when an inbound packet fails to decode.
	OnBadPacket func(error)
	// OnRateLimited is called when a peer exceeds RateLimitMsgsPerMinute.
	OnRateLimited func()
}

func (c *HubConfig) applyDefaults() {
	if c.Name == "" {
		c.Name = "reticulum-go-protocols"
	}
	if c.Version == "" {
		c.Version = "0.1.0"
	}
	if c.Limits.MaxNickBytes == 0 {
		c.Limits.MaxNickBytes = DefaultMaxNickBytes
	}
	if c.Limits.MaxRoomsPerSession == 0 {
		c.Limits.MaxRoomsPerSession = DefaultMaxRoomsPerSession
	}
	if c.Limits.MaxRoomNameBytes == 0 {
		c.Limits.MaxRoomNameBytes = DefaultMaxRoomNameBytes
	}
	if c.Limits.MaxMsgBodyBytes == 0 {
		c.Limits.MaxMsgBodyBytes = DefaultMaxMsgBodyBytes
	}
	if c.Limits.RateLimitMsgsPerMinute == 0 {
		c.Limits.RateLimitMsgsPerMinute = DefaultRateLimitMsgsPerMinute
	}
	if c.MaxResourceBytes == 0 {
		c.MaxResourceBytes = DefaultMaxResourceBytes
	}
}

// NewHubDestination creates an inbound rrc.hub destination.
func NewHubDestination(id *identity.Identity, tr *transport.Transport) (*destination.Destination, error) {
	if id == nil || tr == nil {
		return nil, ErrNilArgument
	}
	dest, err := destination.New(id, destination.In, destination.Single, AppName, tr, HubAspect)
	if err != nil {
		return nil, err
	}
	dest.AcceptsLinks(true)
	return dest, nil
}

type hubPeer struct {
	sess         *session
	peerHash     []byte
	active       atomic.Bool
	rooms        map[string]struct{}
	rateMu       sync.Mutex
	tokens       float64
	lastRefill   time.Time
	pendingHello *Envelope
}

// Hub is an RRC hub that accepts Links and relays room traffic.
type Hub struct {
	mu        sync.Mutex
	tr        *transport.Transport
	dest      *destination.Destination
	id        *identity.Identity
	sender    []byte
	cfg       HubConfig
	peers     map[peerID]*hubPeer
	rooms     map[string]map[peerID]struct{}
	nickIndex map[string]map[peerID]struct{}
	handlers  HubHandlers
	started   bool
}

// NewHub builds a hub bound to dest. Call Start to accept sessions.
func NewHub(tr *transport.Transport, dest *destination.Destination, cfg HubConfig) (*Hub, error) {
	if tr == nil || dest == nil {
		return nil, ErrNilArgument
	}
	cfg.applyDefaults()
	id := dest.GetIdentity()
	if id == nil {
		return nil, ErrNilArgument
	}
	h := &Hub{
		tr:        tr,
		dest:      dest,
		id:        id,
		sender:    append([]byte(nil), id.Hash()...),
		cfg:       cfg,
		peers:     make(map[peerID]*hubPeer),
		rooms:     make(map[string]map[peerID]struct{}),
		nickIndex: make(map[string]map[peerID]struct{}),
		handlers:  cfg.Handlers,
	}
	return h, nil
}

// Start registers the link-established callback and begins accepting sessions.
func (h *Hub) Start() {
	h.mu.Lock()
	if h.started {
		h.mu.Unlock()
		return
	}
	h.started = true
	h.mu.Unlock()

	h.dest.SetLinkEstablishedCallback(func(v any) {
		lnk, ok := v.(*link.Link)
		if !ok || lnk == nil {
			return
		}
		h.acceptLink(lnk)
		lnk.Start()
	})
}

// Destination returns the hub destination.
func (h *Hub) Destination() *destination.Destination {
	return h.dest
}

// DestinationHash returns the hub destination hash.
func (h *Hub) DestinationHash() []byte {
	return h.dest.GetHash()
}

type peerID [IdentityLength]byte

func peerKey(hash []byte) peerID {
	var k peerID
	if len(hash) == IdentityLength {
		copy(k[:], hash)
	}
	return k
}

func (h *Hub) acceptLink(lnk *link.Link) {
	p := &hubPeer{
		rooms:      make(map[string]struct{}),
		tokens:     float64(h.cfg.Limits.RateLimitMsgsPerMinute),
		lastRefill: time.Now(),
	}

	var registerOnce sync.Once
	register := func(hash []byte) {
		if len(hash) != IdentityLength {
			return
		}
		var old *hubPeer
		registerOnce.Do(func() {
			h.mu.Lock()
			p.peerHash = append([]byte(nil), hash...)
			key := peerKey(p.peerHash)
			if prev, ok := h.peers[key]; ok && prev != p {
				old = prev
				for room := range old.rooms {
					if members, ok := h.rooms[room]; ok {
						delete(members, key)
						if len(members) == 0 {
							delete(h.rooms, room)
						}
					}
				}
			}
			h.peers[key] = p
			h.mu.Unlock()
		})
		if old != nil && old.sess != nil {
			old.sess.close()
		}
		if pol := h.cfg.Policy; pol != nil && len(hash) == IdentityLength {
			if err := pol.OnIdentified(hash); err != nil {
				if p.sess != nil {
					_ = p.sess.sendType(TypeError, "", err.Error(), "")
					p.sess.close()
				}
				return
			}
		}
		h.mu.Lock()
		pending := p.pendingHello
		p.pendingHello = nil
		ready := len(p.peerHash) == IdentityLength
		h.mu.Unlock()
		if ready && pending != nil {
			h.handlePeer(p, pending)
		}
	}

	p.sess = newSession(lnk, h.sender, false, func(env *Envelope) {
		defer recoverLog()
		if remote := lnk.GetRemoteIdentity(); remote != nil {
			register(remote.Hash())
		}
		h.mu.Lock()
		ready := len(p.peerHash) == IdentityLength
		if !ready {
			if env.Type == TypeHello {
				p.pendingHello = env
			}
			h.mu.Unlock()
			if remote := lnk.GetRemoteIdentity(); remote != nil {
				register(remote.Hash())
			}
			return
		}
		h.mu.Unlock()
		h.handlePeer(p, env)
	}, func() {
		h.dropPeerIf(p)
	})
	p.sess.onBytes = func(n int) {
		if h.cfg.OnInboundBytes != nil {
			h.cfg.OnInboundBytes(n)
		}
	}
	p.sess.onBad = func(err error) {
		if h.cfg.OnBadPacket != nil {
			h.cfg.OnBadPacket(err)
		}
		_ = h.replyError(p, "bad message")
	}

	lnk.SetRemoteIdentifiedCallback(func(_ *link.Link, id *identity.Identity) {
		if id != nil {
			register(id.Hash())
		}
	})

	if remote := lnk.GetRemoteIdentity(); remote != nil {
		register(remote.Hash())
	}
	if pol := h.cfg.Policy; pol != nil {
		pol.OnLink(lnk)
	}
}

func (h *Hub) dropPeerIf(p *hubPeer) {
	if p == nil || len(p.peerHash) == 0 {
		return
	}
	key := peerKey(p.peerHash)
	h.mu.Lock()
	cur, ok := h.peers[key]
	if !ok || cur != p {
		h.mu.Unlock()
		return
	}
	type roomNotify struct {
		room  string
		peers []*hubPeer
	}
	notes := make([]roomNotify, 0, len(p.rooms))
	nick := ""
	if p.sess != nil {
		nick = p.sess.getNick()
	}
	delete(h.peers, key)
	h.unindexNickLocked(key, nick)
	for room := range p.rooms {
		others := h.roomPeersLocked(room, key)
		if members, ok := h.rooms[room]; ok {
			delete(members, key)
			if len(members) == 0 {
				delete(h.rooms, room)
			}
		}
		notes = append(notes, roomNotify{room: room, peers: others})
	}
	p.rooms = map[string]struct{}{}
	cb := h.handlers.OnClose
	peer := append([]byte(nil), p.peerHash...)
	includeList := h.cfg.IncludeMemberList
	h.mu.Unlock()
	var presenceBody any
	if includeList {
		presenceBody = []any{append([]byte(nil), peer...)}
	}
	for _, n := range notes {
		raw, err := marshalType(h.sender, TypeParted, n.room, presenceBody, nick)
		if err != nil {
			continue
		}
		for _, other := range n.peers {
			_ = other.sess.sendRaw(raw)
		}
	}
	if cb != nil {
		cb(peer)
	}
}

func (h *Hub) applyInboundNick(p *hubPeer, nick string) error {
	nick = SanitizeNick(nick)
	if nick == "" {
		return nil
	}
	if uint64(len(nick)) > h.cfg.Limits.MaxNickBytes {
		return ErrNickTooLong
	}
	old := p.sess.getNick()
	if nick == old {
		return nil
	}
	p.sess.setNick(nick)
	if len(p.peerHash) == IdentityLength {
		h.reindexNick(peerKey(p.peerHash), old, nick)
	}
	return nil
}

func (h *Hub) handlePeer(p *hubPeer, env *Envelope) {
	defer recoverLog()
	if !h.takeToken(p) {
		if h.cfg.OnRateLimited != nil {
			h.cfg.OnRateLimited()
		}
		_ = h.replyError(p, "Rate limit exceeded. Try again later.")
		return
	}

	active := p.active.Load()

	if env.HasNick {
		if err := h.applyInboundNick(p, env.Nick); err != nil {
			_ = h.replyError(p, "nickname too long")
			if !active {
				p.sess.close()
			}
			return
		}
	} else if env.Type == TypeHello {
		if n := HelloLegacyNick(env.Body); n != "" {
			if err := h.applyInboundNick(p, n); err != nil {
				_ = h.replyError(p, "nickname too long")
				if !active {
					p.sess.close()
					return
				}
			}
		}
	}

	if env.Type == TypePong {
		if pol := h.cfg.Policy; pol != nil {
			pol.OnPong(p.peerHash)
		}
		return
	}

	if !active {
		if env.Type != TypeHello {
			_ = h.replyError(p, "send HELLO first")
			return
		}
		h.onHello(p, env)
		return
	}

	switch env.Type {
	case TypeHello:
		h.reHello(p, env)
	case TypeJoin:
		h.onJoin(p, env)
	case TypePart:
		h.onPart(p, env)
	case TypeNotice:
		if h.tryIntercept(p, env) {
			return
		}
		if env.HasDestination {
			h.onDirectNotice(p, env)
			return
		}
		h.dispatchContent(p, env)
	case TypeMsg:
		if h.tryIntercept(p, env) {
			return
		}
		h.dispatchContent(p, env)
	case TypeAction:
		h.dispatchContent(p, env)
	case TypeResourceEnvelope:
		h.onResourceEnvelope(p, env)
	case TypePing:
		h.onPing(p, env)
	case TypeError:
	default:
	}
}

func (h *Hub) onHello(p *hubPeer, env *Envelope) {
	body, _ := ParseHelloBody(env.Body)
	// Nick already applied in handlePeer with length enforcement.

	p.active.Store(true)
	cb := h.handlers.OnHello

	caps := h.cfg.Capabilities
	if caps == nil {
		caps = DefaultHubCapabilities(h.cfg.EnableResourceTransfer)
	}
	wb := &WelcomeBody{
		HubName:      h.cfg.Name,
		HasName:      true,
		HubVersion:   h.cfg.Version,
		HasVersion:   true,
		Capabilities: caps,
		HasCaps:      caps != nil,
		Limits:       h.cfg.Limits,
		HasLimits:    true,
	}
	if err := p.sess.sendType(TypeWelcome, "", wb.ToMap(), ""); err != nil {
		p.sess.close()
		return
	}
	if cb != nil {
		cb(p.peerHash, body, env)
	}
	if pol := h.cfg.Policy; pol != nil {
		pol.AfterWelcome(p.peerHash)
	}
}

func (h *Hub) onJoin(p *hubPeer, env *Envelope) {
	room := NormalizeRoom(env.Room)
	if room == "" {
		_ = h.replyError(p, "missing room")
		return
	}
	if uint64(len(room)) > h.cfg.Limits.MaxRoomNameBytes {
		_ = h.replyError(p, "room name too long")
		return
	}

	if pol := h.cfg.Policy; pol != nil {
		if err := pol.AllowJoin(p.peerHash, room, env.Body); err != nil {
			_ = h.replyError(p, err.Error())
			return
		}
	}

	key := peerKey(p.peerHash)
	h.mu.Lock()
	if _, ok := p.rooms[room]; !ok {
		if uint64(len(p.rooms)) >= h.cfg.Limits.MaxRoomsPerSession {
			h.mu.Unlock()
			_ = h.replyError(p, "room limit exceeded")
			return
		}
		p.rooms[room] = struct{}{}
		if h.rooms[room] == nil {
			h.rooms[room] = make(map[peerID]struct{})
		}
		h.rooms[room][key] = struct{}{}
	}
	var memberList []any
	if h.cfg.IncludeMemberList {
		for pk := range h.rooms[room] {
			if peer, ok := h.peers[pk]; ok {
				memberList = append(memberList, append([]byte(nil), peer.peerHash...))
			}
		}
	}
	cb := h.handlers.OnJoin
	others := h.roomPeersLocked(room, key)
	nick := p.sess.getNick()
	h.mu.Unlock()

	var body any
	if len(memberList) > 0 {
		body = memberList
	}
	var presenceBody any
	if h.cfg.IncludeMemberList {
		presenceBody = []any{append([]byte(nil), p.peerHash...)}
	}
	if len(others) > 0 {
		if raw, err := marshalType(h.sender, TypeJoined, room, presenceBody, nick); err == nil {
			for _, peer := range others {
				_ = peer.sess.sendRaw(raw)
			}
		}
	}
	_ = p.sess.sendType(TypeJoined, room, body, "")
	if cb != nil {
		cb(p.peerHash, room, env)
	}
	if pol := h.cfg.Policy; pol != nil {
		pol.AfterJoin(p.peerHash, room)
	}
}

func (h *Hub) onPart(p *hubPeer, env *Envelope) {
	room := NormalizeRoom(env.Room)
	if room == "" {
		return
	}
	key := peerKey(p.peerHash)
	h.mu.Lock()
	if _, ok := p.rooms[room]; !ok {
		h.mu.Unlock()
		return
	}
	delete(p.rooms, room)
	if members, ok := h.rooms[room]; ok {
		delete(members, key)
		if len(members) == 0 {
			delete(h.rooms, room)
		}
	}
	cb := h.handlers.OnPart
	others := h.roomPeersLocked(room, key)
	nick := p.sess.getNick()
	h.mu.Unlock()

	var presenceBody any
	if h.cfg.IncludeMemberList {
		presenceBody = []any{append([]byte(nil), p.peerHash...)}
	}
	if len(others) > 0 {
		if raw, err := marshalType(h.sender, TypeParted, room, presenceBody, nick); err == nil {
			for _, peer := range others {
				_ = peer.sess.sendRaw(raw)
			}
		}
	}
	_ = p.sess.sendType(TypeParted, room, presenceBody, "")
	if cb != nil {
		cb(p.peerHash, room, env)
	}
	if pol := h.cfg.Policy; pol != nil {
		pol.AfterPart(p.peerHash, room)
	}
}

func (h *Hub) tryIntercept(p *hubPeer, env *Envelope) bool {
	if pol := h.cfg.Policy; pol != nil {
		return pol.Intercept(p.peerHash, env)
	}
	return false
}

func (h *Hub) dispatchContent(p *hubPeer, env *Envelope) {
	if pol := h.cfg.Policy; pol != nil {
		if err := pol.AllowContent(p.peerHash, env); err != nil {
			_ = h.replyError(p, err.Error())
			return
		}
	}
	h.onRoomContent(p, env)
}

func (h *Hub) onRoomContent(p *hubPeer, env *Envelope) {
	room := NormalizeRoom(env.Room)
	if room == "" {
		_ = h.replyError(p, "missing room")
		return
	}

	if env.HasBody && BodySizeBytes(env.Body) > h.cfg.Limits.MaxMsgBodyBytes {
		_ = h.replyError(p, "message body too large")
		return
	}

	h.mu.Lock()
	if _, ok := p.rooms[room]; !ok && h.cfg.Policy == nil {
		h.mu.Unlock()
		_ = h.replyError(p, "not a member of room")
		return
	}
	roomMembers := h.rooms[room]
	members := make([]*hubPeer, 0, len(roomMembers))
	for pk := range roomMembers {
		if peer, ok := h.peers[pk]; ok && peer.active.Load() {
			members = append(members, peer)
		}
	}
	cb := h.handlers.OnMsg
	nick := p.sess.getNick()
	h.mu.Unlock()

	// Always stamp authenticated peer identity. Never trust wire Sender.
	fwd, err := envelopeFrom(env.Type, p.peerHash, env.MsgID, env.Timestamp)
	if err != nil {
		return
	}
	fwd.Room = room
	fwd.HasRoom = true
	if env.HasBody {
		fwd.Body = env.Body
		fwd.HasBody = true
	}
	if nick != "" {
		fwd.Nick = nick
		fwd.HasNick = true
	}

	raw, err := fwd.Marshal()
	if err != nil {
		return
	}
	for _, peer := range members {
		_ = peer.sess.sendRaw(raw)
	}
	if cb != nil {
		cb(p.peerHash, env)
	}
}

func (h *Hub) takeToken(p *hubPeer) bool {
	limit := h.cfg.Limits.RateLimitMsgsPerMinute
	if limit == 0 {
		return true
	}
	now := time.Now()
	p.rateMu.Lock()
	defer p.rateMu.Unlock()
	elapsed := now.Sub(p.lastRefill).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	tokenCap := float64(limit)
	p.tokens += elapsed * (tokenCap / 60.0)
	if p.tokens > tokenCap {
		p.tokens = tokenCap
	}
	p.lastRefill = now
	if p.tokens < 1 {
		return false
	}
	p.tokens--
	return true
}

func (h *Hub) allowRate(p *hubPeer) bool {
	return h.takeToken(p)
}

func (h *Hub) replyError(p *hubPeer, msg string) error {
	return p.sess.sendType(TypeError, "", msg, "")
}

func (h *Hub) roomPeersLocked(room string, exceptKey peerID) []*hubPeer {
	members := h.rooms[room]
	out := make([]*hubPeer, 0, len(members))
	for pk := range members {
		if pk == exceptKey {
			continue
		}
		if peer, ok := h.peers[pk]; ok && peer.active.Load() {
			out = append(out, peer)
		}
	}
	return out
}

func (h *Hub) onDirectNotice(p *hubPeer, env *Envelope) {
	if env.HasRoom {
		_ = h.replyError(p, "direct notice must not include room")
		return
	}
	if !env.HasDestination || len(env.Destination) != IdentityLength {
		_ = h.replyError(p, "direct notice requires destination identity")
		return
	}
	if env.HasBody && BodySizeBytes(env.Body) > h.cfg.Limits.MaxMsgBodyBytes {
		_ = h.replyError(p, "message body too large")
		return
	}

	h.mu.Lock()
	target, ok := h.peers[peerKey(env.Destination)]
	active := ok && target.active.Load()
	nick := p.sess.getNick()
	cb := h.handlers.OnMsg
	h.mu.Unlock()
	if !active {
		_ = h.replyError(p, "destination not connected")
		return
	}

	fwd, err := envelopeFrom(TypeNotice, p.peerHash, env.MsgID, env.Timestamp)
	if err != nil {
		return
	}
	fwd.Destination = cloneBytes(env.Destination)
	fwd.HasDestination = true
	if env.HasBody {
		fwd.Body = env.Body
		fwd.HasBody = true
	}
	if nick != "" {
		fwd.Nick = nick
		fwd.HasNick = true
	}
	_ = target.sess.sendEnvelope(fwd)
	if cb != nil {
		cb(p.peerHash, env)
	}
}

func (h *Hub) onPing(p *hubPeer, env *Envelope) {
	var body any
	if env != nil && env.HasBody {
		body = env.Body
	}
	_ = p.sess.sendType(TypePong, "", body, "")
}

func (h *Hub) onResourceEnvelope(p *hubPeer, env *Envelope) {
	if !h.cfg.EnableResourceTransfer {
		_ = h.replyError(p, "resource transfer disabled")
		return
	}
	body, reason := ValidateResourceEnvelopeBody(env.Body)
	if reason != "" {
		_ = h.replyError(p, reason)
		return
	}
	if body.Size > h.cfg.MaxResourceBytes {
		_ = h.replyError(p, fmt.Sprintf("resource too large: %d > %d", body.Size, h.cfg.MaxResourceBytes))
		return
	}
	if pol := h.cfg.Policy; pol != nil {
		if err := pol.OnResourceEnvelope(p.peerHash, env); err != nil {
			_ = h.replyError(p, err.Error())
			return
		}
	}
	if cb := h.handlers.OnResource; cb != nil {
		cb(p.peerHash, env)
	}
}

// Close tears down all peer sessions.
func (h *Hub) Close() {
	h.mu.Lock()
	peers := make([]*hubPeer, 0, len(h.peers))
	for _, p := range h.peers {
		peers = append(peers, p)
	}
	h.peers = make(map[peerID]*hubPeer)
	h.rooms = make(map[string]map[peerID]struct{})
	h.nickIndex = make(map[string]map[peerID]struct{})
	h.mu.Unlock()
	for _, p := range peers {
		p.sess.close()
	}
}

// PeerCount returns the number of connected peers.
func (h *Hub) PeerCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.peers)
}

// RoomMembers returns identity hashes currently in room.
func (h *Hub) RoomMembers(room string) [][]byte {
	room = NormalizeRoom(room)
	h.mu.Lock()
	defer h.mu.Unlock()
	members := h.rooms[room]
	out := make([][]byte, 0, len(members))
	for pk := range members {
		if p, ok := h.peers[pk]; ok {
			out = append(out, append([]byte(nil), p.peerHash...))
		}
	}
	return out
}

// HasPeer reports whether peerHash is currently connected.
func (h *Hub) HasPeer(peerHash []byte) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.peers[peerKey(peerHash)]
	return ok
}

// FormatError returns a short error description from an ERROR envelope body.
func FormatError(env *Envelope) string {
	if env == nil {
		return ""
	}
	if s, ok := BodyAsString(env.Body); ok {
		return s
	}
	return fmt.Sprint(env.Body)
}
