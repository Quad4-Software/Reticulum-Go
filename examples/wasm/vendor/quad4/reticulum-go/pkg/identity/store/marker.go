// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"quad4/reticulum-go/internal/storage"
)

const (
	markerMagicLen  = 4
	markerHeaderLen = 8
)

var markerMagic = []byte("RSSI")

// IsMarkerPayload reports whether data is an RSSI v1 secret-store marker.
func IsMarkerPayload(data []byte) bool {
	if len(data) < markerHeaderLen {
		return false
	}
	if !bytes.Equal(data[:markerMagicLen], markerMagic) {
		return false
	}
	return data[4] == 1
}

// WriteMarkerFile writes an RSSI v1 marker at path (secret lives in Backend).
func WriteMarkerFile(path string) error {
	buf := make([]byte, markerHeaderLen)
	copy(buf[0:4], markerMagic)
	buf[4] = 1
	return storage.AtomicWriteFile(path, buf, 0o600)
}

// ReadMarkerFile reads and validates an RSSI marker. Returns nil error if valid.
func ReadMarkerFile(path string) error {
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return err
	}
	if !IsMarkerPayload(data) {
		return fmt.Errorf("identity store: not an RSSI marker at %s", path)
	}
	if data[5] != 0 || data[6] != 0 || data[7] != 0 {
		return errorsReserved()
	}
	return nil
}

func errorsReserved() error {
	return fmt.Errorf("identity store: reserved marker header bytes must be zero")
}

// AbsolutePath resolves path for stable Secret Service attributes.
func AbsolutePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("identity store: empty path")
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return abs, nil
}
