// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package storage

import (
	"fmt"
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
