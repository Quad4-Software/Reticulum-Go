// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"sync"
	"time"

	"quad4/reticulum-go/pkg/link"
	"quad4/reticulum-go/pkg/transport"
)

type linkManager struct {
	transport *transport.Transport
	opts      LinkReconnectOptions
	mu        sync.Mutex
	links     map[string]*link.Link
}

func newLinkManager(t *transport.Transport, opts LinkReconnectOptions) *linkManager {
	if opts.Backoff <= 0 {
		opts.Backoff = time.Second
	}
	return &linkManager{
		transport: t,
		opts:      opts,
		links:     make(map[string]*link.Link),
	}
}

func (m *linkManager) Register(l *link.Link) {
	if l == nil || m == nil {
		return
	}
	key := linkKey(l)
	m.mu.Lock()
	m.links[key] = l
	m.mu.Unlock()
	link.WatchAndReconnect(l, m.transport, link.ReconnectPolicy{
		MaxAttempts: m.opts.MaxAttempts,
		Backoff:     m.opts.Backoff,
	})
}

func (m *linkManager) onNetworkAvailable() {
	m.mu.Lock()
	links := make([]*link.Link, 0, len(m.links))
	for _, l := range m.links {
		links = append(links, l)
	}
	m.mu.Unlock()
	for _, l := range links {
		if l.GetStatus() == link.StatusClosed || l.GetStatus() == link.StatusFailed {
			_ = l.Establish()
		}
	}
}

func linkKey(l *link.Link) string {
	if l == nil {
		return ""
	}
	return string(l.DestinationHash())
}
