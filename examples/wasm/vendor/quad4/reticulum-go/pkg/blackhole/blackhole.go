// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package blackhole

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/msgpack/v5/pkg/msgpack/msgpcode"
)

func decodeHex(s string) ([]byte, error) { return hex.DecodeString(s) }
func encodeHex(b []byte) string          { return hex.EncodeToString(b) }

// HashLen is the truncated identity hash length in bytes.
const HashLen = 16

// CheckInterval is the period at which the table sweeps for expired entries.
const CheckInterval = 60 * time.Second

// LocalSource is the bytes value used as the "source" of locally authored
// blackhole entries. It is initialised by SetLocalIdentityHash before
// Add/Remove are called.
var (
	mu                  sync.Mutex
	localIdentityHash   []byte
	enabledSources      map[[HashLen]byte]struct{}
	enabledSourcesAllow bool
)

// SetLocalIdentityHash records the truncated identity hash of the local
// transport. Locally authored entries use this value as their "source" field
func SetLocalIdentityHash(h []byte) {
	mu.Lock()
	defer mu.Unlock()
	if len(h) == HashLen {
		localIdentityHash = append([]byte(nil), h...)
	}
}

// SetEnabledSources installs the optional allow-list of source identity
// hashes that may contribute blackhole entries when LoadAll is called. If
// none are set every non-local source file is loaded, matching behaviour when
// blackhole_sources() returns an empty list (the loader continues without the
// source check).
func SetEnabledSources(sources [][]byte) {
	mu.Lock()
	defer mu.Unlock()
	enabledSources = make(map[[HashLen]byte]struct{}, len(sources))
	for _, s := range sources {
		if len(s) != HashLen {
			continue
		}
		var k [HashLen]byte
		copy(k[:], s)
		enabledSources[k] = struct{}{}
	}
	enabledSourcesAllow = len(enabledSources) > 0
}

// Entry describes a blackhole record. Until is a Unix timestamp in seconds
// (wall-clock seconds, same convention as time.Time.Unix()), or 0 when the
// entry never expires.
type Entry struct {
	Source []byte
	Until  float64
	Reason string
}

// Table is the in-memory list of blackholed identity hashes plus disk
// persistence helpers. Table is safe for concurrent use.
type Table struct {
	mu      sync.RWMutex
	entries map[[HashLen]byte]Entry
	dir     string
	now     func() time.Time
}

// New returns an empty Table whose persistence directory is dir. dir is
// created on demand by the Persist / LoadAll calls, aligned with
// Reticulum.blackholepath.
func New(dir string) *Table {
	return &Table{
		entries: make(map[[HashLen]byte]Entry),
		dir:     dir,
		now:     time.Now,
	}
}

// Dir returns the persistence directory the Table writes local entries to.
func (t *Table) Dir() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.dir
}

// SetClock overrides the time source. Used by tests.
func (t *Table) SetClock(now func() time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if now != nil {
		t.now = now
	}
}

// Has reports whether identityHash is currently blackholed. Expired entries
// are reported as not blackholed, regardless of whether they have been
// swept yet.
func (t *Table) Has(identityHash []byte) bool {
	if len(identityHash) != HashLen {
		return false
	}
	var k [HashLen]byte
	copy(k[:], identityHash)
	t.mu.RLock()
	defer t.mu.RUnlock()
	e, ok := t.entries[k]
	if !ok {
		return false
	}
	if e.Until == 0 {
		return true
	}
	return float64(t.now().Unix()) < e.Until
}

// Add inserts a locally-authored blackhole entry and persists the local
// table to disk. A zero until disables expiry. reason is an opaque string,
// kept identical to the wire field. Add returns true when the entry was
// inserted, false when it was already present.
func (t *Table) Add(identityHash []byte, until float64, reason string) (bool, error) {
	if len(identityHash) != HashLen {
		return false, fmt.Errorf("blackhole: identity hash must be %d bytes", HashLen)
	}
	mu.Lock()
	src := append([]byte(nil), localIdentityHash...)
	mu.Unlock()
	if len(src) != HashLen {
		return false, errors.New("blackhole: SetLocalIdentityHash has not been called")
	}
	var k [HashLen]byte
	copy(k[:], identityHash)
	t.mu.Lock()
	if _, exists := t.entries[k]; exists {
		t.mu.Unlock()
		return false, nil
	}
	t.entries[k] = Entry{Source: src, Until: until, Reason: reason}
	t.mu.Unlock()
	if err := t.PersistLocal(); err != nil {
		return true, err
	}
	return true, nil
}

// Remove drops identityHash from the table. Returns true when the entry was
// present.
func (t *Table) Remove(identityHash []byte) (bool, error) {
	if len(identityHash) != HashLen {
		return false, fmt.Errorf("blackhole: identity hash must be %d bytes", HashLen)
	}
	var k [HashLen]byte
	copy(k[:], identityHash)
	t.mu.Lock()
	_, exists := t.entries[k]
	if exists {
		delete(t.entries, k)
	}
	t.mu.Unlock()
	if !exists {
		return false, nil
	}
	if err := t.PersistLocal(); err != nil {
		return true, err
	}
	return true, nil
}

// All returns a deterministic snapshot of all entries currently held.
func (t *Table) All() map[[HashLen]byte]Entry {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[[HashLen]byte]Entry, len(t.entries))
	for k, v := range t.entries {
		cp := v
		cp.Source = append([]byte(nil), v.Source...)
		out[k] = cp
	}
	return out
}

// Snapshot returns the table contents as a sorted slice keyed by the raw
// identity hash. Useful for deterministic iteration in tests and request
// handlers.
func (t *Table) Snapshot() []SnapshotEntry {
	all := t.All()
	out := make([]SnapshotEntry, 0, len(all))
	for k, v := range all {
		out = append(out, SnapshotEntry{Hash: append([]byte(nil), k[:]...), Entry: v})
	}
	sort.Slice(out, func(i, j int) bool {
		for x := range HashLen {
			if out[i].Hash[x] != out[j].Hash[x] {
				return out[i].Hash[x] < out[j].Hash[x]
			}
		}
		return false
	})
	return out
}

// SnapshotEntry pairs a hash with its entry for deterministic iteration.
type SnapshotEntry struct {
	Hash  []byte
	Entry Entry
}

// SweepExpired removes expired entries. Returns the number of removed
// entries. Callers should call SweepExpired periodically (CheckInterval).
func (t *Table) SweepExpired() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := float64(t.now().Unix())
	removed := 0
	for k, e := range t.entries {
		if e.Until != 0 && now > e.Until {
			delete(t.entries, k)
			removed++
		}
	}
	return removed
}

// PersistLocal writes the local-source subset of the table to <dir>/local
// in the msgpack layout used by persist_blackhole.
func (t *Table) PersistLocal() error {
	t.mu.RLock()
	dir := t.dir
	mu.Lock()
	src := append([]byte(nil), localIdentityHash...)
	mu.Unlock()
	local := make(map[string]map[string]any)
	for k, e := range t.entries {
		if !equalBytes(e.Source, src) {
			continue
		}
		entry := map[string]any{}
		if e.Until != 0 {
			entry["until"] = e.Until
		} else {
			entry["until"] = nil
		}
		if e.Reason != "" {
			entry["reason"] = e.Reason
		} else {
			entry["reason"] = nil
		}
		entry["source"] = e.Source
		local[string(k[:])] = entry
	}
	t.mu.RUnlock()

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	packed, err := encodeBlackholeMap(local)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "local")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, packed, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadAll loads every blackhole source file in <dir>. Files named "local"
// are credited to the locally-stored identity hash. Every other file name

// must be a hex-encoded source identity hash. When SetEnabledSources has
// been called with a non-empty list, source files for sources not on the
// list are skipped, matching Reticulum.blackhole_sources().
func (t *Table) LoadAll() error {
	t.mu.Lock()
	dir := t.dir
	t.mu.Unlock()
	if dir == "" {
		return nil
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	mu.Lock()
	localHash := append([]byte(nil), localIdentityHash...)
	allow := enabledSourcesAllow
	allowSet := enabledSources
	mu.Unlock()

	now := float64(t.now().Unix())
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if !isSafeBlackholeName(name) {
			continue
		}
		var srcHash []byte
		if name == "local" {
			srcHash = localHash
		} else {
			b, err := decodeHex(name)
			if err != nil || len(b) != HashLen {
				continue
			}
			if allow {
				var key [HashLen]byte
				copy(key[:], b)
				if _, ok := allowSet[key]; !ok {
					continue
				}
			}
			srcHash = b
		}
		raw, err := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 -- name is validated by isSafeBlackholeName above
		if err != nil {
			continue
		}
		decoded, err := DecodeBlackholeMap(raw)
		if err != nil {
			continue
		}
		t.mu.Lock()
		for hashStr, entry := range decoded {
			if len(hashStr) != HashLen {
				continue
			}
			var k [HashLen]byte
			copy(k[:], hashStr)
			if cur, ok := t.entries[k]; ok && equalBytes(cur.Source, localHash) {
				continue
			}
			if entry.Until != 0 && now >= entry.Until {
				continue
			}
			if entry.Source == nil {
				entry.Source = srcHash
			}
			t.entries[k] = entry
		}
		t.mu.Unlock()
	}
	return nil
}

// MergeRemote merges entries received from another transport instance
// identified by sourceHash. Local entries (source == localIdentityHash) are
// never overwritten. The resulting table is persisted to disk under the
// source's per-source file (<dir>/<source_hex>) so that subsequent LoadAll
// calls re-pick up the same entries.
func (t *Table) MergeRemote(sourceHash []byte, decoded map[string]Entry) error {
	if len(sourceHash) != HashLen {
		return fmt.Errorf("blackhole: source hash must be %d bytes", HashLen)
	}
	mu.Lock()
	localHash := append([]byte(nil), localIdentityHash...)
	mu.Unlock()
	now := float64(t.now().Unix())
	t.mu.Lock()
	for hashStr, entry := range decoded {
		if len(hashStr) != HashLen {
			continue
		}
		var k [HashLen]byte
		copy(k[:], hashStr)
		if cur, ok := t.entries[k]; ok && equalBytes(cur.Source, localHash) {
			continue
		}
		if entry.Until != 0 && now >= entry.Until {
			continue
		}
		if entry.Source == nil {
			entry.Source = append([]byte(nil), sourceHash...)
		}
		t.entries[k] = entry
	}
	t.mu.Unlock()

	t.mu.RLock()
	dir := t.dir
	t.mu.RUnlock()
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	out := make(map[string]map[string]any, len(decoded))
	for hashStr, entry := range decoded {
		if len(hashStr) != HashLen {
			continue
		}
		entry := entry
		if entry.Source == nil {
			entry.Source = append([]byte(nil), sourceHash...)
		}
		out[hashStr] = entryToMsgpackMap(entry)
	}
	packed, err := encodeBlackholeMap(out)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, encodeHex(sourceHash))
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, packed, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// EncodeForRequest builds the msgpack payload that a /list request handler
// returns to a peer.
func (t *Table) EncodeForRequest() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]map[string]any, len(t.entries))
	for k, e := range t.entries {
		out[string(k[:])] = entryToMsgpackMap(e)
	}
	return encodeBlackholeMap(out)
}

// MaxDecodeSize bounds the size of a msgpack payload accepted by
// DecodeBlackholeMap. A blackhole list with even tens of thousands of
// entries fits comfortably below this. The cap exists to defeat

// adversarial inputs that declare a length far larger than the buffer
// they actually carry.
const MaxDecodeSize = 4 * 1024 * 1024

// MaxEntries bounds the number of entries we accept in a single
// payload. The wire stack does not enforce a hard cap, but the practical upper
// bound is <<10k. Anything larger almost certainly indicates an

// attempt to allocate an unreasonable amount of memory.
const MaxEntries = 65535

// DecodeBlackholeMap decodes a msgpack payload produced by
// persist_blackhole or blackhole_list_handler into a map keyed by the raw
// 16-byte identity hash (as a string) to its Entry.
//
// The decoder bounds map and slice lengths to defeat adversarial inputs
// that declare a declared-length far larger than the available payload
// (the msgpack decoder does not bound decodeSlice allocations).
func DecodeBlackholeMap(raw []byte) (map[string]Entry, error) {
	if len(raw) == 0 {
		return map[string]Entry{}, nil
	}
	if len(raw) > MaxDecodeSize {
		return nil, fmt.Errorf("blackhole: payload too large (%d > %d bytes)", len(raw), MaxDecodeSize)
	}

	dec := msgpack.NewDecoder(bytes.NewReader(raw))
	dec.UseLooseInterfaceDecoding(true)

	mapLen, err := dec.DecodeMapLen()
	if err != nil {
		return nil, fmt.Errorf("blackhole: decode map len: %w", err)
	}
	if mapLen < 0 {
		return map[string]Entry{}, nil
	}
	if mapLen > MaxEntries || mapLen > len(raw) {
		return nil, fmt.Errorf("blackhole: map length %d exceeds bounds (max=%d, payload=%d)", mapLen, MaxEntries, len(raw))
	}

	out := make(map[string]Entry, mapLen)
	for i := range mapLen {
		key, err := decodeBytesOrString(dec, len(raw))
		if err != nil {
			return nil, fmt.Errorf("blackhole: decode key %d: %w", i, err)
		}
		entry, err := decodeEntry(dec, len(raw))
		if err != nil {
			return nil, fmt.Errorf("blackhole: decode entry %d: %w", i, err)
		}
		out[string(key)] = entry
	}
	return out, nil
}

// peekIsBytesOrString reports whether the next code is a bin or str type
// that decodeBytesOrString can safely consume.
func peekIsBytesOrString(c byte) bool {
	switch {
	case c >= msgpcode.FixedStrLow && c <= msgpcode.FixedStrHigh:
		return true
	case c == msgpcode.Str8 || c == msgpcode.Str16 || c == msgpcode.Str32:
		return true
	case c == msgpcode.Bin8 || c == msgpcode.Bin16 || c == msgpcode.Bin32:
		return true
	}
	return false
}

func decodeBytesOrString(dec *msgpack.Decoder, maxLen int) ([]byte, error) {
	c, err := dec.PeekCode()
	if err != nil {
		return nil, err
	}
	if c == msgpcode.Nil {
		_ = dec.DecodeNil()
		return nil, nil
	}
	if !peekIsBytesOrString(c) {
		return nil, fmt.Errorf("blackhole: unexpected msgpack code 0x%02x for bin/str value", c)
	}
	n, err := dec.DecodeBytesLen()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	if n > maxLen {
		return nil, fmt.Errorf("blackhole: declared bin/str length %d exceeds payload size %d", n, maxLen)
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(dec.Buffered(), b); err != nil {
		return nil, fmt.Errorf("blackhole: read bin/str payload: %w", err)
	}
	return b, nil
}

// decodeNumeric pulls out a float64 from a numeric or nil value. Anything
// else returns ok=false so the caller can decide whether to treat it as a
// missing entry.
func decodeNumeric(dec *msgpack.Decoder) (val float64, ok bool, err error) {
	c, err := dec.PeekCode()
	if err != nil {
		return 0, false, err
	}
	if c == msgpcode.Nil {
		_ = dec.DecodeNil()
		return 0, false, nil
	}
	switch {
	case c == msgpcode.Float || c == msgpcode.Double:
		f, err := dec.DecodeFloat64()
		return f, err == nil, err
	case c == msgpcode.Int8 || c == msgpcode.Int16 || c == msgpcode.Int32 || c == msgpcode.Int64:
		i, err := dec.DecodeInt64()
		return float64(i), err == nil, err
	case c == msgpcode.Uint8 || c == msgpcode.Uint16 || c == msgpcode.Uint32 || c == msgpcode.Uint64:
		u, err := dec.DecodeUint64()
		return float64(u), err == nil, err
	case c <= msgpcode.PosFixedNumHigh:
		i, err := dec.DecodeInt64()
		return float64(i), err == nil, err
	case c >= msgpcode.NegFixedNumLow:
		i, err := dec.DecodeInt64()
		return float64(i), err == nil, err
	}
	if err := dec.Skip(); err != nil {
		return 0, false, err
	}
	return 0, false, nil
}

func decodeEntry(dec *msgpack.Decoder, maxLen int) (Entry, error) {
	subLen, err := dec.DecodeMapLen()
	if err != nil {
		return Entry{}, fmt.Errorf("decode submap len: %w", err)
	}
	if subLen < 0 {
		return Entry{}, nil
	}
	if subLen > 32 || subLen > maxLen {
		return Entry{}, fmt.Errorf("submap length %d exceeds bounds", subLen)
	}
	entry := Entry{}
	for i := range subLen {
		key, err := dec.DecodeString()
		if err != nil {
			return Entry{}, fmt.Errorf("decode submap key %d: %w", i, err)
		}
		switch key {
		case "source":
			b, err := decodeBytesOrString(dec, maxLen)
			if err != nil {
				return Entry{}, fmt.Errorf("decode source: %w", err)
			}
			entry.Source = b
		case "until":
			val, ok, err := decodeNumeric(dec)
			if err != nil {
				return Entry{}, fmt.Errorf("decode until: %w", err)
			}
			if ok {
				entry.Until = val
			}
		case "reason":
			c, err := dec.PeekCode()
			if err != nil {
				return Entry{}, fmt.Errorf("peek reason: %w", err)
			}
			if c == msgpcode.Nil {
				_ = dec.DecodeNil()
				continue
			}
			if !peekIsBytesOrString(c) {
				if err := dec.Skip(); err != nil {
					return Entry{}, fmt.Errorf("skip reason: %w", err)
				}
				continue
			}
			b, err := dec.DecodeBytes()
			if err != nil {
				return Entry{}, fmt.Errorf("decode reason: %w", err)
			}
			if len(b) > maxLen {
				return Entry{}, fmt.Errorf("reason length %d exceeds payload size", len(b))
			}
			entry.Reason = string(b)
		default:
			if err := dec.Skip(); err != nil {
				return Entry{}, fmt.Errorf("skip unknown key %q: %w", key, err)
			}
		}
	}
	return entry, nil
}

func entryToMsgpackMap(e Entry) map[string]any {
	out := map[string]any{
		"source": e.Source,
	}
	if e.Until != 0 {
		out["until"] = e.Until
	} else {
		out["until"] = nil
	}
	if e.Reason != "" {
		out["reason"] = e.Reason
	} else {
		out["reason"] = nil
	}
	return out
}

// encodeBlackholeMap encodes the map so identity-hash keys are emitted as
// msgpack bin.
func encodeBlackholeMap(in map[string]map[string]any) ([]byte, error) {
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	if err := enc.EncodeMapLen(len(keys)); err != nil {
		return nil, err
	}
	for _, k := range keys {
		if err := enc.EncodeBytes([]byte(k)); err != nil {
			return nil, err
		}
		if err := enc.Encode(in[k]); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// isSafeBlackholeName validates that a directory entry can safely be opened
// as a blackhole source file. Allowed names are exactly "local" or a
// 32-character lowercase-hex source identity hash.
func isSafeBlackholeName(name string) bool {
	if name == "local" {
		return true
	}
	if len(name) != HashLen*2 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
