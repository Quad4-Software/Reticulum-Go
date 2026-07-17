// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"sync"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/link"
)

// session groups one identity with the destinations, links, and pending
// requests owned by it, plus the WebSocket clients currently attached to
// its event stream. Sessions let multiple independent applications share
// one control API server without their destinations or event streams
// colliding.
type session struct {
	id       string
	identity *identity.Identity

	mu           sync.RWMutex
	destinations map[string]*destination.Destination // key: hex destination hash

	linksMu sync.RWMutex
	links   map[string]*linkSession // key: hex link ID

	pendingMu       sync.Mutex
	pendingRequests map[string]chan any // key: hex request ID

	clientsMu sync.Mutex
	clients   map[*wsClient]struct{}
}

// linkSession tracks one link.Link alongside the bookkeeping the control
// API needs to translate its callbacks into WebSocket events. idHex and
// established are guarded by establishedMu because they are read from
// link.Link's closed callback, which may run while the Link's own mutex is
// held (see newOutboundLinkCallbacks in link.go): they must be cheap,
// lock-only reads that never call back into the Link itself.
type linkSession struct {
	link          *link.Link
	establishedMu sync.Mutex
	idHex         string
	established   bool
}

func newSession(id string, ident *identity.Identity) *session {
	return &session{
		id:              id,
		identity:        ident,
		destinations:    make(map[string]*destination.Destination),
		links:           make(map[string]*linkSession),
		pendingRequests: make(map[string]chan any),
		clients:         make(map[*wsClient]struct{}),
	}
}

func (s *session) addDestination(hashHex string, dest *destination.Destination) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.destinations[hashHex] = dest
}

func (s *session) destination(hashHex string) (*destination.Destination, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dest, ok := s.destinations[hashHex]
	return dest, ok
}

func (s *session) addLink(idHex string, ls *linkSession) {
	s.linksMu.Lock()
	defer s.linksMu.Unlock()
	s.links[idHex] = ls
}

func (s *session) getLink(idHex string) (*linkSession, bool) {
	s.linksMu.RLock()
	defer s.linksMu.RUnlock()
	ls, ok := s.links[idHex]
	return ls, ok
}

func (s *session) removeLink(idHex string) {
	s.linksMu.Lock()
	defer s.linksMu.Unlock()
	delete(s.links, idHex)
}

// awaitResponse registers a channel for requestIDHex and returns it.
// The request handler bridge blocks on it until deliverResponse sends data
// or the caller's own timeout elapses.
func (s *session) awaitResponse(requestIDHex string) chan any {
	ch := make(chan any, 1)
	s.pendingMu.Lock()
	s.pendingRequests[requestIDHex] = ch
	s.pendingMu.Unlock()
	return ch
}

// deliverResponse hands data to the goroutine blocked in awaitResponse for
// requestIDHex, if one is still waiting. It reports whether a waiter was
// found.
func (s *session) deliverResponse(requestIDHex string, data any) bool {
	s.pendingMu.Lock()
	ch, ok := s.pendingRequests[requestIDHex]
	if ok {
		delete(s.pendingRequests, requestIDHex)
	}
	s.pendingMu.Unlock()
	if !ok {
		return false
	}
	ch <- data
	return true
}

// forgetResponse removes requestIDHex's waiter without delivering data,
// used once awaitResponse's own timeout fires.
func (s *session) forgetResponse(requestIDHex string) {
	s.pendingMu.Lock()
	delete(s.pendingRequests, requestIDHex)
	s.pendingMu.Unlock()
}

func (s *session) addClient(c *wsClient) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	s.clients[c] = struct{}{}
}

func (s *session) removeClient(c *wsClient) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	delete(s.clients, c)
}

// broadcast delivers v to every WebSocket client currently attached to the
// session, encoding it once and reusing the encoded form for all of them.
func (s *session) broadcast(v any) {
	s.clientsMu.Lock()
	clients := make([]*wsClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.clientsMu.Unlock()

	for _, c := range clients {
		c.send(v)
	}
}

// close disconnects every WebSocket client attached to the session and
// tears down any links it opened or accepted. Destinations registered with
// the transport are not unregistered: the transport does not expose a
// removal path, matching every other embed mode in this codebase (see
// cmd/reticulum-go and examples/pageserver).
func (s *session) close() {
	s.clientsMu.Lock()
	clients := make([]*wsClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.clientsMu.Unlock()

	for _, c := range clients {
		c.close()
	}

	s.linksMu.Lock()
	links := make([]*link.Link, 0, len(s.links))
	for _, ls := range s.links {
		links = append(links, ls.link)
	}
	s.links = make(map[string]*linkSession)
	s.linksMu.Unlock()

	for _, l := range links {
		l.Teardown()
	}
}
