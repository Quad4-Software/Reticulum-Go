// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
)

// persistedIface is one iface baseline row in the learning store.
type persistedIface struct {
	EwmaPPS float64 `msgpack:"ewma_pps"`
	EwmaBPS float64 `msgpack:"ewma_bps"`
	Samples int     `msgpack:"samples"`
	Ready   bool    `msgpack:"ready"`
}

// persistedStore is the on-disk learning document.
type persistedStore struct {
	Version     int                       `msgpack:"version"`
	Fingerprint string                    `msgpack:"fingerprint"`
	Promoted    bool                      `msgpack:"promoted"`
	SavedAtUnix int64                     `msgpack:"saved_at"`
	Ifaces      map[string]persistedIface `msgpack:"ifaces"`
}

func networkFingerprint(names []string) string {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	h := sha256.New()
	for _, n := range sorted {
		_, _ = h.Write([]byte(n))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}

func loadStore(path string) (*persistedStore, error) {
	if path == "" {
		return nil, nil
	}
	path = filepath.Clean(path)
	data, err := os.ReadFile(path) // #nosec G304 -- path is transport storage dir plus fixed StoreFileName
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var st persistedStore
	if err := msgpack.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("dos_protect store: %w", err)
	}
	if st.Version != StoreVersion {
		return nil, nil
	}
	if st.Ifaces == nil {
		st.Ifaces = make(map[string]persistedIface)
	}
	return &st, nil
}

func saveStore(path string, st *persistedStore) error {
	if path == "" || st == nil {
		return nil
	}
	path = filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	st.Version = StoreVersion
	st.SavedAtUnix = time.Now().Unix()
	data, err := msgpack.Marshal(st)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { // #nosec G304 -- path is transport storage dir plus fixed StoreFileName
		return err
	}
	return os.Rename(tmp, path)
}
