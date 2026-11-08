// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

// Package sqlitestore provides the RNS 1.6.0 SQLite persistence layer
// (storage/known_destinations.db and cache/announces.db). The driver is
// vendored modernc.org/sqlite (pure Go, no cgo); builds that cannot host
// it (js/wasip1/tinygo) get ErrUnavailable stubs and callers fall back to
// the legacy file persistence.
package sqlitestore

import "errors"

// ErrUnavailable is returned when the sqlite driver is unavailable in
// this build (js/wasip1/tinygo) or a database could not be opened; callers
// must fall back to legacy file persistence.
var ErrUnavailable = errors.New("sqlitestore: sqlite backend unavailable")

// KnownDestEntry is one row of the known_destinations table (Python
// Storage.__entry_row layout).
type KnownDestEntry struct {
	DestinationHash []byte
	LastHeard       float64
	PacketHash      []byte
	PublicKey       []byte
	AppData         []byte
	Used            float64
	Stamp           []byte
	StampValue      float64
	LRCost          *float64
}
