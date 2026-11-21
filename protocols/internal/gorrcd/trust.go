// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"sort"
	"sync"
)

type Trust struct {
	mu      sync.RWMutex
	trusted map[ID]struct{}
	banned  map[ID]struct{}
}

func NewTrust() *Trust {
	return &Trust{
		trusted: make(map[ID]struct{}),
		banned:  make(map[ID]struct{}),
	}
}

func (t *Trust) Load(trusted, banned []string) error {
	nt := make(map[ID]struct{})
	nb := make(map[ID]struct{})
	for _, s := range trusted {
		id, err := parseFullID(s)
		if err != nil {
			return err
		}
		nt[id] = struct{}{}
	}
	for _, s := range banned {
		id, err := parseFullID(s)
		if err != nil {
			return err
		}
		nb[id] = struct{}{}
	}
	t.mu.Lock()
	t.trusted = nt
	t.banned = nb
	t.mu.Unlock()
	return nil
}

func (t *Trust) IsTrusted(id ID) bool {
	t.mu.RLock()
	_, ok := t.trusted[id]
	t.mu.RUnlock()
	return ok
}

func (t *Trust) IsBanned(id ID) bool {
	t.mu.RLock()
	_, ok := t.banned[id]
	t.mu.RUnlock()
	return ok
}

func (t *Trust) Ban(id ID) {
	t.mu.Lock()
	t.banned[id] = struct{}{}
	t.mu.Unlock()
}

func (t *Trust) Unban(id ID) bool {
	t.mu.Lock()
	_, ok := t.banned[id]
	delete(t.banned, id)
	t.mu.Unlock()
	return ok
}

func (t *Trust) BannedHex() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]string, 0, len(t.banned))
	for id := range t.banned {
		out = append(out, id.Hex())
	}
	sort.Strings(out)
	return out
}

func (t *Trust) Counts() (trusted, banned int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.trusted), len(t.banned)
}
