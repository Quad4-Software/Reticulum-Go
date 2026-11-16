// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"strings"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
)

func (h *Hub) reHello(p *hubPeer, env *Envelope) {
	h.mu.Lock()
	rooms := make([]string, 0, len(p.rooms))
	for room := range p.rooms {
		rooms = append(rooms, room)
	}
	h.mu.Unlock()
	for _, room := range rooms {
		fake := &Envelope{Room: room, HasRoom: true}
		h.onPart(p, fake)
	}
	p.active.Store(false)
	h.onHello(p, env)
}

func (h *Hub) reindexNick(id peerID, oldNick, newNick string) {
	h.mu.Lock()
	h.unindexNickLocked(id, oldNick)
	n := strings.ToLower(strings.TrimSpace(newNick))
	if n != "" {
		m := h.nickIndex[n]
		if m == nil {
			m = make(map[peerID]struct{})
			h.nickIndex[n] = m
		}
		m[id] = struct{}{}
	}
	h.mu.Unlock()
}

func (h *Hub) unindexNickLocked(id peerID, nick string) {
	n := strings.ToLower(strings.TrimSpace(nick))
	if n == "" {
		return
	}
	m := h.nickIndex[n]
	if m == nil {
		return
	}
	delete(m, id)
	if len(m) == 0 {
		delete(h.nickIndex, n)
	}
}

func (h *Hub) peerLocked(hash []byte) *hubPeer {
	p, ok := h.peers[peerKey(hash)]
	if !ok || p == nil || !p.active.Load() {
		return nil
	}
	return p
}

// ApplyRuntime updates name, limits, and feature flags after /reload.
func (h *Hub) ApplyRuntime(name, version string, includeMembers, enableRes bool, maxRes uint64, limits HubLimits) {
	h.mu.Lock()
	if name != "" {
		h.cfg.Name = name
	}
	if version != "" {
		h.cfg.Version = version
	}
	h.cfg.IncludeMemberList = includeMembers
	h.cfg.EnableResourceTransfer = enableRes
	if maxRes > 0 {
		h.cfg.MaxResourceBytes = maxRes
	}
	h.cfg.Limits = limits
	h.mu.Unlock()
}

// SendNotice delivers a hub NOTICE to one peer.
func (h *Hub) SendNotice(peer []byte, room, text string) error {
	h.mu.Lock()
	p := h.peerLocked(peer)
	h.mu.Unlock()
	if p == nil {
		return ErrSessionClosed
	}
	return p.sess.sendType(TypeNotice, room, text, "")
}

// SendError delivers a hub ERROR to one peer.
func (h *Hub) SendError(peer []byte, room, text string) error {
	h.mu.Lock()
	p := h.peerLocked(peer)
	h.mu.Unlock()
	if p == nil {
		return ErrSessionClosed
	}
	return p.sess.sendType(TypeError, room, text, "")
}

// SendPing sends a hub-initiated PING.
func (h *Hub) SendPing(peer []byte, body any) error {
	h.mu.Lock()
	p := h.peerLocked(peer)
	h.mu.Unlock()
	if p == nil {
		return ErrSessionClosed
	}
	return p.sess.sendType(TypePing, "", body, "")
}

// BroadcastNotice sends NOTICE to every member of room.
func (h *Hub) BroadcastNotice(room, text string) {
	room = NormalizeRoom(room)
	h.mu.Lock()
	peers := h.roomPeersLocked(room, peerID{})
	h.mu.Unlock()
	if len(peers) == 0 {
		return
	}
	raw, err := marshalType(h.sender, TypeNotice, room, text, "")
	if err != nil {
		return
	}
	for _, p := range peers {
		_ = p.sess.sendRaw(raw)
	}
}

// RemoveFromRoom drops membership without sending PART from the client.
func (h *Hub) RemoveFromRoom(peer []byte, room string) error {
	room = NormalizeRoom(room)
	key := peerKey(peer)
	h.mu.Lock()
	p, ok := h.peers[key]
	if !ok || p == nil {
		h.mu.Unlock()
		return ErrSessionClosed
	}
	if _, in := p.rooms[room]; !in {
		h.mu.Unlock()
		return ErrNotMember
	}
	delete(p.rooms, room)
	if members, ok := h.rooms[room]; ok {
		delete(members, key)
		if len(members) == 0 {
			delete(h.rooms, room)
		}
	}
	h.mu.Unlock()
	return nil
}

// Disconnect tears down the peer link.
func (h *Hub) Disconnect(peer []byte) {
	h.mu.Lock()
	p := h.peers[peerKey(peer)]
	h.mu.Unlock()
	if p != nil && p.sess != nil {
		p.sess.close()
	}
}

// PeerLink returns the live link for peer, or nil.
func (h *Hub) PeerLink(peer []byte) *link.Link {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.peers[peerKey(peer)]
	if p == nil || p.sess == nil {
		return nil
	}
	return p.sess.lnk
}

// Nick returns the advisory nickname for peer.
func (h *Hub) Nick(peer []byte) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.peers[peerKey(peer)]
	if p == nil || p.sess == nil {
		return ""
	}
	return p.sess.getNick()
}

// IsMember reports whether peer is in room.
func (h *Hub) IsMember(peer []byte, room string) bool {
	room = NormalizeRoom(room)
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.peers[peerKey(peer)]
	if p == nil {
		return false
	}
	_, ok := p.rooms[room]
	return ok
}

// RoomMemberInfo returns nick and hash for each member of room.
func (h *Hub) RoomMemberInfo(room string) []PeerInfo {
	room = NormalizeRoom(room)
	h.mu.Lock()
	defer h.mu.Unlock()
	members := h.rooms[room]
	out := make([]PeerInfo, 0, len(members))
	for pk := range members {
		p, ok := h.peers[pk]
		if !ok {
			continue
		}
		nick := ""
		if p.sess != nil {
			nick = p.sess.getNick()
		}
		out = append(out, PeerInfo{Hash: append([]byte(nil), p.peerHash...), Nick: nick})
	}
	return out
}

// AllPeers returns connected identified peers.
func (h *Hub) AllPeers() []PeerInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]PeerInfo, 0, len(h.peers))
	for _, p := range h.peers {
		nick := ""
		if p.sess != nil {
			nick = p.sess.getNick()
		}
		out = append(out, PeerInfo{Hash: append([]byte(nil), p.peerHash...), Nick: nick})
	}
	return out
}

// WelcomedCount returns peers that completed HELLO.
func (h *Hub) WelcomedCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, p := range h.peers {
		if p.active.Load() {
			n++
		}
	}
	return n
}

// OccupiedRooms returns live room names and member counts.
func (h *Hub) OccupiedRooms() []struct {
	Name  string
	Count int
} {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]struct {
		Name  string
		Count int
	}, 0, len(h.rooms))
	for name, members := range h.rooms {
		out = append(out, struct {
			Name  string
			Count int
		}{Name: name, Count: len(members)})
	}
	return out
}

// LookupNick returns peers whose nickname matches (case insensitive).
func (h *Hub) LookupNick(nick string) [][]byte {
	key := strings.ToLower(strings.TrimSpace(nick))
	h.mu.Lock()
	defer h.mu.Unlock()
	ids := h.nickIndex[key]
	out := make([][]byte, 0, len(ids))
	for id := range ids {
		if p, ok := h.peers[id]; ok {
			out = append(out, append([]byte(nil), p.peerHash...))
		}
	}
	return out
}

// LookupHashPrefix returns peers whose identity hash starts with prefix.
func (h *Hub) LookupHashPrefix(prefix []byte) [][]byte {
	if len(prefix) == 0 {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([][]byte, 0)
	for id, p := range h.peers {
		if len(prefix) <= IdentityLength && bytesHasPrefix(id[:], prefix) {
			out = append(out, append([]byte(nil), p.peerHash...))
		}
	}
	return out
}

// ActivePeerHashes returns hashes of welcomed peers.
func (h *Hub) ActivePeerHashes() [][]byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([][]byte, 0, len(h.peers))
	for _, p := range h.peers {
		if p.active.Load() {
			out = append(out, append([]byte(nil), p.peerHash...))
		}
	}
	return out
}

func bytesHasPrefix(b, prefix []byte) bool {
	if len(prefix) > len(b) {
		return false
	}
	for i := range prefix {
		if b[i] != prefix[i] {
			return false
		}
	}
	return true
}
