// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build js || wasip1 || tinygo || !((darwin && (amd64 || arm64)) || (freebsd && (386 || amd64 || arm || arm64)) || (illumos && amd64) || (linux && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || s390x)) || (netbsd && amd64) || (openbsd && (amd64 || arm64)) || windows)

package sqlitestore

const Available = false

type KnownDestinationsStore struct{}
type AnnounceCache struct{}

func OpenKnownDestinations(dbPath string, hashWidth int, readOnly bool) (*KnownDestinationsStore, error) {
	return nil, ErrUnavailable
}
func (s *KnownDestinationsStore) MarkDirty(e *KnownDestEntry)               {}
func (s *KnownDestinationsStore) MarkDeleted(destHash []byte)               {}
func (s *KnownDestinationsStore) MarkRatchetDirty(destHash, ratchet []byte) {}
func (s *KnownDestinationsStore) MarkRatchetDeleted(destHash []byte)        {}
func (s *KnownDestinationsStore) LoadAll() ([]*KnownDestEntry, error)       { return nil, ErrUnavailable }
func (s *KnownDestinationsStore) Flush() error                              { return ErrUnavailable }
func (s *KnownDestinationsStore) FetchRatchet(destHash []byte) ([]byte, error) {
	return nil, ErrUnavailable
}
func (s *KnownDestinationsStore) LoadRatchets() (map[string][]byte, error) {
	return nil, ErrUnavailable
}
func (s *KnownDestinationsStore) ExpireRatchets(olderThan float64) (int64, error) {
	return 0, ErrUnavailable
}
func (s *KnownDestinationsStore) Checkpoint()  {}
func (s *KnownDestinationsStore) Close() error { return nil }

func OpenAnnounceCache(dbPath string) (*AnnounceCache, error)           { return nil, ErrUnavailable }
func (c *AnnounceCache) Store(packetHash, raw []byte, iface string)     {}
func (c *AnnounceCache) Fetch(packetHash []byte) ([]byte, string, bool) { return nil, "", false }
func (c *AnnounceCache) PendingCount() int                              { return 0 }
func (c *AnnounceCache) Flush() (int, error)                            { return 0, ErrUnavailable }
func (c *AnnounceCache) Clean(activeHashes [][]byte) (int64, error)     { return 0, ErrUnavailable }
func (c *AnnounceCache) ImportLegacy(dirPath string) (int, error)       { return 0, ErrUnavailable }
func (c *AnnounceCache) Close() error                                   { return nil }
