// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const defaultDataDirName = ".reticulum-go"

// DataDir returns the storage directory for path tables, known destinations,
// transport identity, and related on-disk state. Resolution order:
// RETICULUM_STORAGE_PATH, directory adjacent to configPath, then
// ~/.reticulum-go/storage.
func DataDir(configPath string) (string, error) {
	if env := os.Getenv("RETICULUM_STORAGE_PATH"); env != "" {
		return env, nil
	}
	if configPath != "" {
		return filepath.Join(filepath.Dir(configPath), "storage"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, defaultDataDirName, "storage"), nil
}

// DestinationTablePath returns the path table snapshot file path.
func DestinationTablePath(configPath string) (string, error) {
	dir, err := DataDir(configPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "destination_table"), nil
}

// RatchetsDir returns the directory for announced peer ratchet public keys.
func RatchetsDir(configPath string) (string, error) {
	dir, err := DataDir(configPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ratchets"), nil
}

// KnownDestinationsPath returns the known destinations snapshot file path.
func KnownDestinationsPath(configPath string) (string, error) {
	dir, err := DataDir(configPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "known_destinations"), nil
}

// EnsureDataDir creates the storage directory with restrictive permissions.
func EnsureDataDir(configPath string) (string, error) {
	dir, err := DataDir(configPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create storage directory %q: %w", dir, err)
	}
	return dir, nil
}

// ReadFileCapped reads a file but refuses data above max bytes, so state
// loads stay bounded when a file is replaced by something oversized.
func ReadFileCapped(path string, max int64) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- callers own the path contract
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("file %q exceeds size cap of %d bytes", path, max)
	}
	return data, nil
}
