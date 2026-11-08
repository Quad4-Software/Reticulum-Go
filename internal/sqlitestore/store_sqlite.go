// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !js && !wasip1 && !tinygo

package sqlitestore

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// Available reports whether the sqlite driver is compiled in.
const Available = true

// KnownDestinationsStore mirrors Python RNS.Storage.KnownDestinationsStore:
// write-behind batching with dirty tracking, identity_hash =
// sha256(public_key)[:hashWidth].
type KnownDestinationsStore struct {
	db        *sql.DB
	readOnly  bool
	hashWidth int

	mu            sync.Mutex
	dirtyUpserts  map[string]*KnownDestEntry
	dirtyDeletes  map[string]struct{}
	ratchetUpsert map[string][]byte
	ratchetDelete map[string]struct{}
}

// OpenKnownDestinations opens (or creates) the known_destinations DB. When
// readOnly is true it opens the file URI in mode=ro like Python shared-
// instance clients.
func OpenKnownDestinations(dbPath string, hashWidth int, readOnly bool) (*KnownDestinationsStore, error) {
	var db *sql.DB
	var err error
	if readOnly {
		db, err = sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", dbPath))
	} else {
		db, err = sql.Open("sqlite", dbPath)
	}
	if err != nil {
		return nil, err
	}
	s := &KnownDestinationsStore{
		db: db, readOnly: readOnly, hashWidth: hashWidth,
		dirtyUpserts: make(map[string]*KnownDestEntry), dirtyDeletes: make(map[string]struct{}),
		ratchetUpsert: make(map[string][]byte), ratchetDelete: make(map[string]struct{}),
	}
	if !readOnly {
		for _, p := range []string{
			"PRAGMA journal_mode=WAL", "PRAGMA synchronous=NORMAL", "PRAGMA journal_size_limit=8388608",
		} {
			if _, err := db.Exec(p); err != nil {
				db.Close()
				return nil, err
			}
		}
		if err := s.createSchema(); err != nil {
			db.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *KnownDestinationsStore) createSchema() error {
	var v int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	// Pre-release cleanup: rebuild release-format schema (Python 1.6.0).
	if v > 0 {
		if _, err := s.db.Exec("DROP TABLE IF EXISTS known_destinations"); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS known_destinations (
		destination_hash BLOB PRIMARY KEY,
		last_heard       REAL NOT NULL,
		packet_hash      BLOB,
		public_key       BLOB NOT NULL,
		app_data         BLOB,
		used             REAL NOT NULL DEFAULT 0,
		stamp            BLOB,
		stamp_value      REAL,
		lr_cost          REAL,
		identity_hash    BLOB NOT NULL)`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS ratchets (
		destination_hash BLOB PRIMARY KEY,
		ratchet          BLOB NOT NULL,
		received         REAL NOT NULL)`); err != nil {
		return err
	}
	if _, err := s.db.Exec("PRAGMA user_version = 0"); err != nil {
		return err
	}
	return nil
}

func (s *KnownDestinationsStore) identityHash(publicKey []byte) []byte {
	h := sha256.Sum256(publicKey)
	return h[:s.hashWidth]
}

// MarkDirty records an entry for write-behind persistence.
func (s *KnownDestinationsStore) MarkDirty(e *KnownDestEntry) {
	if s.readOnly {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := string(e.DestinationHash)
	delete(s.dirtyDeletes, k)
	cp := *e
	s.dirtyUpserts[k] = &cp
}

// MarkDeleted records a destination removal.
func (s *KnownDestinationsStore) MarkDeleted(destHash []byte) {
	if s.readOnly {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := string(destHash)
	delete(s.dirtyUpserts, k)
	s.dirtyDeletes[k] = struct{}{}
}

// MarkRatchetDirty / MarkRatchetDeleted queue ratchet changes.
func (s *KnownDestinationsStore) MarkRatchetDirty(destHash, ratchet []byte) {
	if s.readOnly {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := string(destHash)
	delete(s.ratchetDelete, k)
	s.ratchetUpsert[k] = append([]byte(nil), ratchet...)
}

func (s *KnownDestinationsStore) MarkRatchetDeleted(destHash []byte) {
	if s.readOnly {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := string(destHash)
	delete(s.ratchetUpsert, k)
	s.ratchetDelete[k] = struct{}{}
}

// LoadAll reads every known destination (Python Storage.load_all).
func (s *KnownDestinationsStore) LoadAll() ([]*KnownDestEntry, error) {
	rows, err := s.db.Query("SELECT destination_hash,last_heard,packet_hash,public_key,app_data,used,stamp,stamp_value,lr_cost FROM known_destinations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*KnownDestEntry
	for rows.Next() {
		e := &KnownDestEntry{}
		var stamp sql.RawBytes
		var lr sql.NullFloat64
		var stampVal sql.NullFloat64
		var pktHash, appData []byte
		if err := rows.Scan(&e.DestinationHash, &e.LastHeard, &pktHash, &e.PublicKey, &appData, &e.Used, &stamp, &stampVal, &lr); err != nil {
			return nil, err
		}
		e.PacketHash, e.AppData = pktHash, appData
		e.Stamp = stamp
		e.StampValue = stampVal.Float64
		if lr.Valid {
			v := lr.Float64
			e.LRCost = &v
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Flush writes all pending changes in one transaction (Python flush).
func (s *KnownDestinationsStore) Flush() error {
	if s.readOnly {
		return nil
	}
	s.mu.Lock()
	upserts, deletes := s.dirtyUpserts, s.dirtyDeletes
	ru, rd := s.ratchetUpsert, s.ratchetDelete
	s.dirtyUpserts = make(map[string]*KnownDestEntry)
	s.dirtyDeletes = make(map[string]struct{})
	s.ratchetUpsert = make(map[string][]byte)
	s.ratchetDelete = make(map[string]struct{})
	s.mu.Unlock()

	if len(upserts)+len(deletes)+len(ru)+len(rd) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, e := range upserts {
		var lr any
		if e.LRCost != nil {
			lr = *e.LRCost
		}
		if _, err := tx.Exec(`INSERT INTO known_destinations
			(destination_hash,last_heard,packet_hash,public_key,app_data,used,stamp,stamp_value,lr_cost,identity_hash)
			VALUES (?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(destination_hash) DO UPDATE SET last_heard=excluded.last_heard,packet_hash=excluded.packet_hash,public_key=excluded.public_key,app_data=excluded.app_data,used=excluded.used,stamp=excluded.stamp,stamp_value=excluded.stamp_value,lr_cost=excluded.lr_cost,identity_hash=excluded.identity_hash`,
			e.DestinationHash, e.LastHeard, e.PacketHash, e.PublicKey, e.AppData, e.Used, e.Stamp, e.StampValue, lr, s.identityHash(e.PublicKey)); err != nil {
			tx.Rollback()
			return err
		}
	}
	for k := range deletes {
		if _, err := tx.Exec("DELETE FROM known_destinations WHERE destination_hash=?", []byte(k)); err != nil {
			tx.Rollback()
			return err
		}
	}
	for k, r := range ru {
		if _, err := tx.Exec(`INSERT INTO ratchets (destination_hash,ratchet,received) VALUES (?,?,?)
			ON CONFLICT(destination_hash) DO UPDATE SET ratchet=excluded.ratchet,received=excluded.received`,
			[]byte(k), r, float64Now()); err != nil {
			tx.Rollback()
			return err
		}
	}
	for k := range rd {
		if _, err := tx.Exec("DELETE FROM ratchets WHERE destination_hash=?", []byte(k)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// FetchRatchet returns the stored ratchet for a destination, if any.
func (s *KnownDestinationsStore) FetchRatchet(destHash []byte) ([]byte, error) {
	row := s.db.QueryRow("SELECT ratchet FROM ratchets WHERE destination_hash=?", destHash)
	var r []byte
	if err := row.Scan(&r); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return r, nil
}

// LoadRatchets returns all ratchets.
func (s *KnownDestinationsStore) LoadRatchets() (map[string][]byte, error) {
	rows, err := s.db.Query("SELECT destination_hash,ratchet FROM ratchets")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]byte)
	for rows.Next() {
		var h, r []byte
		if err := rows.Scan(&h, &r); err != nil {
			return nil, err
		}
		out[string(h)] = r
	}
	return out, rows.Err()
}

// DeleteRatchets removes rows; ExpireRatchets deletes rows older than age seconds.
func (s *KnownDestinationsStore) ExpireRatchets(olderThan float64) (int64, error) {
	res, err := s.db.Exec("DELETE FROM ratchets WHERE received < ?", olderThan)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Checkpoint runs a WAL checkpoint so read-only readers see fresh data.
func (s *KnownDestinationsStore) Checkpoint() {
	_, _ = s.db.Exec("PRAGMA wal_checkpoint(FULL)")
}

func (s *KnownDestinationsStore) Close() error { return s.db.Close() }

// AnnounceCache mirrors Python RNS.Storage.AnnounceCache.
type AnnounceCache struct {
	db      *sql.DB
	mu      sync.Mutex
	pending map[string]cachedAnnounce
}

type cachedAnnounce struct {
	received float64
	raw      []byte
	iface    string
}

func OpenAnnounceCache(dbPath string) (*AnnounceCache, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	for _, p := range []string{
		"PRAGMA auto_vacuum=INCREMENTAL", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=NORMAL",
		"PRAGMA journal_size_limit=8388608",
		`CREATE TABLE IF NOT EXISTS cached_announces (
			packet_hash         BLOB PRIMARY KEY,
			received            REAL NOT NULL,
			raw                 BLOB NOT NULL,
			interface_reference TEXT)`,
		"PRAGMA user_version = 1",
	} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &AnnounceCache{db: db, pending: make(map[string]cachedAnnounce)}, nil
}

func (c *AnnounceCache) Store(packetHash, raw []byte, iface string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending[string(packetHash)] = cachedAnnounce{received: float64Now(), raw: append([]byte(nil), raw...), iface: iface}
}

func (c *AnnounceCache) Fetch(packetHash []byte) (raw []byte, iface string, ok bool) {
	c.mu.Lock()
	if p, hit := c.pending[string(packetHash)]; hit {
		c.mu.Unlock()
		return p.raw, p.iface, true
	}
	c.mu.Unlock()
	row := c.db.QueryRow("SELECT raw, interface_reference FROM cached_announces WHERE packet_hash=?", packetHash)
	var i sql.NullString
	if err := row.Scan(&raw, &i); err != nil {
		return nil, "", false
	}
	return raw, i.String, true
}

func (c *AnnounceCache) PendingCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}

func (c *AnnounceCache) Flush() (int, error) {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[string]cachedAnnounce)
	c.mu.Unlock()
	if len(pending) == 0 {
		return 0, nil
	}
	tx, err := c.db.Begin()
	if err != nil {
		return 0, err
	}
	for h, e := range pending {
		var iface any
		if e.iface != "" {
			iface = e.iface
		}
		if _, err := tx.Exec(`INSERT INTO cached_announces VALUES (?,?,?,?)
			ON CONFLICT(packet_hash) DO UPDATE SET received=excluded.received, raw=excluded.raw, interface_reference=excluded.interface_reference`,
			[]byte(h), e.received, e.raw, iface); err != nil {
			tx.Rollback()
			c.mu.Lock()
			for k, v := range pending {
				c.pending[k] = v
			}
			c.mu.Unlock()
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(pending), nil
}

// Clean removes cached announces not in activeHashes (Python clean).
func (c *AnnounceCache) Clean(activeHashes [][]byte) (int64, error) {
	if _, err := c.Flush(); err != nil {
		return 0, err
	}
	tx, err := c.db.Begin()
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec("DROP TABLE IF EXISTS temp.active_announces"); err != nil {
		tx.Rollback()
		return 0, err
	}
	if _, err := tx.Exec("CREATE TEMP TABLE active_announces (packet_hash BLOB PRIMARY KEY)"); err != nil {
		tx.Rollback()
		return 0, err
	}
	for _, h := range activeHashes {
		if _, err := tx.Exec("INSERT OR IGNORE INTO active_announces VALUES (?)", h); err != nil {
			tx.Rollback()
			return 0, err
		}
	}
	res, err := tx.Exec("DELETE FROM cached_announces WHERE packet_hash NOT IN (SELECT packet_hash FROM active_announces)")
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	removed, _ := res.RowsAffected()
	if removed > 0 {
		_, _ = c.db.Exec("PRAGMA incremental_vacuum")
	}
	return removed, nil
}

// ImportLegacy reads legacy announce files from dir into the cache and
// deletes them (Python import_legacy).
func (c *AnnounceCache) ImportLegacy(dirPath string) (int, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dirPath, e.Name()))
		if err != nil || len(raw) == 0 {
			continue
		}
		h := sha256.Sum256(raw)
		c.Store(h[:16], raw, "")
		n++
	}
	if _, err := c.Flush(); err != nil {
		return n, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		_ = os.Remove(filepath.Join(dirPath, e.Name()))
	}
	return n, nil
}

func (c *AnnounceCache) Close() error { return c.db.Close() }
