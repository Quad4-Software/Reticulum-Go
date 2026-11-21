// SPDX-License-Identifier: LicenseRef-Reticulum
package phonebook

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

const (
	AllowAll  byte = 0xFF
	AllowNone byte = 0xFE
)

const (
	HashLen     = 16
	FullHashLen = 32
)

// Entry is a named identity in the phonebook.
type Entry struct {
	Name  string
	Hash  []byte
	Alias string
}

// Book stores contacts and caller policy.
type Book struct {
	mutex   sync.RWMutex
	entries []Entry
	byName  map[string]int
	byAlias map[string]int
	allowed [][]byte
	blocked [][]byte
	policy  byte
}

func New() *Book {
	return &Book{
		byName:  map[string]int{},
		byAlias: map[string]int{},
		policy:  AllowAll,
	}
}

func (b *Book) Policy() byte {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.policy
}

func (b *Book) SetPolicy(policy byte) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if policy == AllowNone || policy == AllowAll {
		b.policy = policy
		return
	}
	b.policy = 0
}

func (b *Book) SetAllowed(hashes [][]byte) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.allowed = cloneHashes(hashes)
	b.policy = 0
}

func (b *Book) SetBlocked(hashes [][]byte) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.blocked = cloneHashes(hashes)
}

func (b *Book) Add(e Entry) error {
	if len(e.Hash) == 0 {
		return fmt.Errorf("empty identity hash")
	}
	name := strings.TrimSpace(e.Name)
	if name == "" {
		return fmt.Errorf("empty name")
	}
	b.mutex.Lock()
	defer b.mutex.Unlock()
	e.Name = name
	e.Hash = append([]byte(nil), e.Hash...)
	e.Alias = strings.TrimSpace(e.Alias)
	if _, ok := b.byName[strings.ToLower(name)]; ok {
		return fmt.Errorf("duplicate name %s", name)
	}
	idx := len(b.entries)
	b.entries = append(b.entries, e)
	b.byName[strings.ToLower(name)] = idx
	if e.Alias != "" {
		b.byAlias[e.Alias] = idx
	}
	return nil
}

func (b *Book) Entries() []Entry {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	out := make([]Entry, len(b.entries))
	copy(out, b.entries)
	return out
}

func (b *Book) Lookup(nameOrAlias string) (Entry, bool) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	key := strings.TrimSpace(nameOrAlias)
	if i, ok := b.byName[strings.ToLower(key)]; ok {
		return b.entries[i], true
	}
	if i, ok := b.byAlias[key]; ok {
		return b.entries[i], true
	}
	return Entry{}, false
}

func (b *Book) AllowedHashes() [][]byte {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	if b.policy == AllowAll || b.policy == AllowNone {
		return nil
	}
	if len(b.allowed) > 0 {
		return cloneHashes(b.allowed)
	}
	out := make([][]byte, 0, len(b.entries))
	for _, e := range b.entries {
		out = append(out, append([]byte(nil), e.Hash...))
	}
	return out
}

func (b *Book) BlockedHashes() [][]byte {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return cloneHashes(b.blocked)
}

func (b *Book) AllowPhonebook() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.policy = 0
	b.allowed = nil
}

func (b *Book) IsAllowed(hash []byte) bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	if containsHash(b.blocked, hash) {
		return false
	}
	if b.policy == AllowAll {
		return true
	}
	if b.policy == AllowNone {
		return false
	}
	if len(b.allowed) > 0 {
		return containsHash(b.allowed, hash)
	}
	for _, e := range b.entries {
		if bytes.Equal(e.Hash, hash) {
			return true
		}
	}
	return false
}

func ParseHash(s string) ([]byte, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, " ", "")
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		s = s[1 : len(s)-1]
	}
	raw, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(raw) != HashLen && len(raw) != FullHashLen {
		return nil, fmt.Errorf("identity hash must be 16 or 32 bytes")
	}
	if len(raw) == FullHashLen {
		return raw[:HashLen], nil
	}
	return raw, nil
}

func cloneHashes(in [][]byte) [][]byte {
	out := make([][]byte, len(in))
	for i, h := range in {
		out[i] = append([]byte(nil), h...)
	}
	return out
}

func containsHash(list [][]byte, hash []byte) bool {
	for _, h := range list {
		if bytes.Equal(h, hash) {
			return true
		}
	}
	return false
}
