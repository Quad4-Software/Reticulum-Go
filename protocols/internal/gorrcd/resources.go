// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"crypto/rand"
	"crypto/sha256"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/resource"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

type expectation struct {
	id       []byte
	kind     string
	size     uint64
	sha256   []byte
	encoding string
	room     string
	expires  time.Time
}

type Resources struct {
	mu     sync.Mutex
	svc    *Service
	byPeer map[ID]map[string]*expectation
}

func NewResources(svc *Service) *Resources {
	return &Resources{svc: svc, byPeer: make(map[ID]map[string]*expectation)}
}

func (r *Resources) OnLink(lnk *link.Link) {
	if r.svc == nil || !r.svc.cfg.EnableResourceTransfer || lnk == nil {
		return
	}
	_ = lnk.SetResourceStrategy(link.AcceptApp)
	lnk.SetResourceCallback(func(v any) bool {
		return r.advertised(lnk, v)
	})
	lnk.SetResourceConcludedCallback(func(v any) {
		r.concluded(lnk, v)
	})
}

func (r *Resources) Add(peer []byte, env *rrc.Envelope) error {
	body, reason := rrc.ValidateResourceEnvelopeBody(env.Body)
	if reason != "" {
		return errString(reason)
	}
	id, ok := idFrom(peer)
	if !ok {
		return errString("invalid peer")
	}
	room := ""
	if env.HasRoom {
		room = env.Room
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(id)
	m := r.byPeer[id]
	if m == nil {
		m = make(map[string]*expectation)
		r.byPeer[id] = m
	}
	max := r.svc.cfg.MaxPendingResourceExpectations
	if max <= 0 {
		max = 8
	}
	if len(m) >= max {
		return errString("too many pending resource expectations")
	}
	ttl := r.svc.cfg.ResourceExpectationTTLS
	if ttl <= 0 {
		ttl = 30
	}
	key := string(body.ID)
	m[key] = &expectation{
		id:       append([]byte(nil), body.ID...),
		kind:     body.Kind,
		size:     body.Size,
		sha256:   append([]byte(nil), body.SHA256...),
		encoding: body.Encoding,
		room:     room,
		expires:  time.Now().Add(time.Duration(ttl * float64(time.Second))),
	}
	return nil
}

func (r *Resources) advertised(lnk *link.Link, v any) bool {
	if !r.svc.cfg.EnableResourceTransfer {
		r.svc.stats.Inc("resources_rejected", 1)
		return false
	}
	adv, ok := v.(*resource.ResourceAdvertisement)
	if !ok || adv == nil {
		r.svc.stats.Inc("resources_rejected", 1)
		return false
	}
	if adv.DataSize < 0 {
		r.svc.stats.Inc("resources_rejected", 1)
		return false
	}
	dataSize := uint64(adv.DataSize) // #nosec G115 -- DataSize checked non-negative
	if dataSize > r.svc.cfg.MaxResourceBytes {
		r.svc.stats.Inc("resources_rejected", 1)
		return false
	}
	peer := r.peerOf(lnk)
	id, ok := idFrom(peer)
	if !ok {
		r.svc.stats.Inc("resources_rejected", 1)
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(id)
	if r.matchSizeLocked(id, dataSize) == nil {
		r.svc.stats.Inc("resources_rejected", 1)
		return false
	}
	return true
}

func (r *Resources) concluded(lnk *link.Link, v any) {
	var payload []byte
	switch x := v.(type) {
	case link.IncomingResource:
		payload = x.Data
	case *link.IncomingResource:
		if x != nil {
			payload = x.Data
		}
	case []byte:
		payload = x
	default:
		return
	}
	peer := r.peerOf(lnk)
	id, ok := idFrom(peer)
	if !ok {
		return
	}
	sum := sha256.Sum256(payload)
	payloadLen := uint64(len(payload)) // #nosec G115 -- len is non-negative
	r.mu.Lock()
	exp := r.matchLocked(id, payloadLen, sum[:])
	if exp != nil {
		delete(r.byPeer[id], string(exp.id))
	}
	r.mu.Unlock()
	if exp == nil {
		return
	}
	if len(exp.sha256) > 0 && subtleNeq(exp.sha256, sum[:]) {
		return
	}
	r.svc.stats.Inc("resources_received", 1)
	r.svc.stats.Inc("resource_bytes_received", payloadLen)
	if exp.kind == rrc.ResourceKindNotice && exp.room != "" && r.svc.hub != nil {
		text := string(payload)
		r.svc.hub.BroadcastNotice(exp.room, text)
		r.svc.stats.Inc("notices_forwarded", 1)
	}
}

func (r *Resources) Send(peer []byte, kind string, payload []byte, room, encoding string) bool {
	if r.svc.hub == nil || !r.svc.cfg.EnableResourceTransfer {
		return false
	}
	payloadLen := uint64(len(payload)) // #nosec G115 -- len is non-negative
	if payloadLen > r.svc.cfg.MaxResourceBytes {
		return false
	}
	lnk := r.svc.hub.PeerLink(peer)
	if lnk == nil {
		return false
	}
	rid := make([]byte, 8)
	if _, err := rand.Read(rid); err != nil {
		return false
	}
	sum := sha256.Sum256(payload)
	body := &rrc.ResourceEnvelopeBody{
		ID: rid, Kind: kind, Size: payloadLen, SHA256: sum[:], Encoding: encoding,
		HasID: true, HasKind: true, HasSize: true, HasHash: true, HasEnc: encoding != "",
	}
	env, err := rrc.NewEnvelope(rrc.TypeResourceEnvelope, r.svc.sender)
	if err != nil {
		return false
	}
	if room != "" {
		env.Room = room
		env.HasRoom = true
	}
	env.Body = body.ToMap()
	env.HasBody = true
	raw, err := env.Marshal()
	if err != nil {
		return false
	}
	if err := lnk.SendPacket(raw); err != nil {
		return false
	}
	res, err := resource.New(payload, false)
	if err != nil {
		return false
	}
	if err := lnk.SendResource(res); err != nil {
		return false
	}
	r.svc.stats.Inc("resources_sent", 1)
	r.svc.stats.Inc("resource_bytes_sent", payloadLen)
	return true
}

func (r *Resources) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id := range r.byPeer {
		r.expireLocked(id)
	}
}

func (r *Resources) Clear() {
	r.mu.Lock()
	r.byPeer = make(map[ID]map[string]*expectation)
	r.mu.Unlock()
}

func (r *Resources) peerOf(lnk *link.Link) []byte {
	if r.svc.hub == nil || lnk == nil {
		return nil
	}
	for _, p := range r.svc.hub.AllPeers() {
		if r.svc.hub.PeerLink(p.Hash) == lnk {
			return p.Hash
		}
	}
	return nil
}

func (r *Resources) expireLocked(id ID) {
	m := r.byPeer[id]
	if m == nil {
		return
	}
	now := time.Now()
	for k, e := range m {
		if now.After(e.expires) {
			delete(m, k)
		}
	}
	if len(m) == 0 {
		delete(r.byPeer, id)
	}
}

func (r *Resources) matchSizeLocked(id ID, size uint64) *expectation {
	for _, e := range r.byPeer[id] {
		if e.size == size {
			return e
		}
	}
	return nil
}

func (r *Resources) matchLocked(id ID, size uint64, sum []byte) *expectation {
	for _, e := range r.byPeer[id] {
		if e.size != size {
			continue
		}
		if len(e.sha256) > 0 && subtleNeq(e.sha256, sum) {
			continue
		}
		return e
	}
	return nil
}

type stringError string

func (e stringError) Error() string { return string(e) }

func errString(s string) error { return stringError(s) }

func subtleNeq(a, b []byte) bool {
	if len(a) != len(b) {
		return true
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v != 0
}
