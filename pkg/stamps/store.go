// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package stamps

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/internal/storage"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

const (
	destinationStampExtension = ".dstamp"
	stampFileVersion          = 1
)

type stampFile struct {
	V         int     `msgpack:"v"`
	Stamp     []byte  `msgpack:"stamp"`
	Value     float64 `msgpack:"value"`
	Generated float64 `msgpack:"generated"`
}

// StampsDir returns <storagePath>/stamps, matching Stamps.stamps_dir.
func StampsDir(storagePath string) string {
	if storagePath == "" {
		return ""
	}
	return filepath.Join(storagePath, "stamps")
}

// DestinationStampFile returns the persistence path for a destination stamp,
// <storagePath>/stamps/<hexhash>.dstamp.
func DestinationStampFile(storagePath string, destinationHash []byte) string {
	dir := StampsDir(storagePath)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, hex.EncodeToString(destinationHash)+destinationStampExtension)
}

// StoreDestinationStamp persists a destination stamp as a python-compatible
// msgpack file via an atomic write, matching Stamps.store_destination_stamp.
func StoreDestinationStamp(storagePath string, destinationHash, stamp []byte, value float64) error {
	path := DestinationStampFile(storagePath, destinationHash)
	if path == "" {
		return errors.New("stamps: no storage path available")
	}
	data, err := msgpack.Marshal(stampFile{
		V:         stampFileVersion,
		Stamp:     stamp,
		Value:     value,
		Generated: float64(time.Now().UnixNano()) / 1e9,
	})
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(path, data, 0o600)
}

// LoadDestinationStamp loads and revalidates a stored destination stamp,
// matching Stamps.load_destination_stamp: material is the workblock
// (expand_rounds=0) and the stamp must satisfy requiredValue. Returns
// (stamp, value, true) on success.
func LoadDestinationStamp(storagePath string, destinationHash, material []byte, requiredValue float64) ([]byte, float64, bool) {
	path := DestinationStampFile(storagePath, destinationHash)
	if path == "" {
		return nil, 0, false
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- path is derived from the configured storage dir and a hashed destination key
	if err != nil {
		return nil, 0, false
	}
	var entry stampFile
	if err := msgpack.Unmarshal(raw, &entry); err != nil {
		return nil, 0, false
	}
	if entry.V != stampFileVersion || len(entry.Stamp) != StampLength {
		return nil, 0, false
	}
	workblock, err := StampWorkblock(material, 0)
	if err != nil {
		return nil, 0, false
	}
	if !StampValid(entry.Stamp, requiredValue, workblock) {
		return nil, 0, false
	}
	return entry.Stamp, StampValue(workblock, entry.Stamp), true
}
