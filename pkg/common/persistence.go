// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"os"
	"strconv"
	"strings"
)

const (
	// DefaultMaxInMemoryPaths is the soft path-table cap under in-memory storage.
	DefaultMaxInMemoryPaths = 100_000

	// DefaultMaxInMemoryKnownDestinations is the soft known-dest cap under
	// in-memory storage.
	DefaultMaxInMemoryKnownDestinations = 100_000

	// DefaultMaxInMemoryResourceBytes is the soft split-resource staging budget
	// under in-memory storage (256 MiB).
	DefaultMaxInMemoryResourceBytes = 256 << 20
)

// ApplyPersistenceEnv overrides persistence-related config from environment
// variables. Set RETICULUM_IN_MEMORY_PATH_TABLE=1 or
// RETICULUM_IN_MEMORY_KNOWN_DESTINATIONS=1 to force in-memory tables.
// RETICULUM_IN_MEMORY_STORAGE=1 forces fully ephemeral storage.
// RETICULUM_SOFT_MEMORY_LIMIT accepts a byte count or K/M/G suffix.
func (c *ReticulumConfig) ApplyPersistenceEnv() {
	if c == nil {
		return
	}
	if envBool("RETICULUM_IN_MEMORY_PATH_TABLE") {
		c.InMemoryPathTable = true
	}
	if envBool("RETICULUM_IN_MEMORY_KNOWN_DESTINATIONS") {
		c.InMemoryKnownDestinations = true
	}
	if envBool("RETICULUM_IN_MEMORY_STORAGE") {
		c.InMemoryStorage = true
	}
	if v := strings.TrimSpace(os.Getenv("RETICULUM_SOFT_MEMORY_LIMIT")); v != "" {
		if n, err := ParseByteSize(v); err == nil && n > 0 {
			c.SoftMemoryLimitBytes = n
		}
	}
	c.NormalizeInMemoryFlags()
}

// NormalizeInMemoryFlags propagates InMemoryStorage onto the per-table flags.
func (c *ReticulumConfig) NormalizeInMemoryFlags() {
	if c == nil {
		return
	}
	if c.InMemoryStorage {
		c.InMemoryPathTable = true
		c.InMemoryKnownDestinations = true
	}
}

// UseInMemoryStorage reports whether stack state must stay off disk.
// True when InMemoryStorage is set, or when neither ConfigPath nor
// RETICULUM_STORAGE_PATH is available (library and test defaults).
func (c *ReticulumConfig) UseInMemoryStorage() bool {
	if c == nil {
		return true
	}
	if c.InMemoryStorage {
		return true
	}
	if c.ConfigPath == "" && strings.TrimSpace(os.Getenv("RETICULUM_STORAGE_PATH")) == "" {
		return true
	}
	return false
}

// EffectiveMaxInMemoryPaths returns the path-table cap when InMemoryStorage
// is explicitly enabled. Auto ephemeral mode (empty ConfigPath) does not
// install a cap. Returns 0 when the cap is disabled.
func (c *ReticulumConfig) EffectiveMaxInMemoryPaths() int {
	if c == nil || !c.InMemoryStorage {
		return 0
	}
	if c.MaxInMemoryPaths < 0 {
		return 0
	}
	if c.MaxInMemoryPaths == 0 {
		return DefaultMaxInMemoryPaths
	}
	return c.MaxInMemoryPaths
}

// EffectiveMaxInMemoryKnownDestinations returns the known-dest cap when
// InMemoryStorage is explicitly enabled.
func (c *ReticulumConfig) EffectiveMaxInMemoryKnownDestinations() int {
	if c == nil || !c.InMemoryStorage {
		return 0
	}
	if c.MaxInMemoryKnownDestinations < 0 {
		return 0
	}
	if c.MaxInMemoryKnownDestinations == 0 {
		return DefaultMaxInMemoryKnownDestinations
	}
	return c.MaxInMemoryKnownDestinations
}

// EffectiveMaxInMemoryResourceBytes returns the split-resource staging budget
// when InMemoryStorage is explicitly enabled.
func (c *ReticulumConfig) EffectiveMaxInMemoryResourceBytes() int64 {
	if c == nil || !c.InMemoryStorage {
		return 0
	}
	if c.MaxInMemoryResourceBytes < 0 {
		return 0
	}
	if c.MaxInMemoryResourceBytes == 0 {
		return DefaultMaxInMemoryResourceBytes
	}
	return c.MaxInMemoryResourceBytes
}

// ParseByteSize parses a decimal byte count with an optional K, M, or G suffix
// (1024-based). Empty or invalid input returns an error.
func ParseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, strconv.ErrSyntax
	}
	mult := int64(1)
	switch {
	case strings.HasSuffix(strings.ToLower(s), "k"):
		mult = 1 << 10
		s = strings.TrimSpace(s[:len(s)-1])
	case strings.HasSuffix(strings.ToLower(s), "m"):
		mult = 1 << 20
		s = strings.TrimSpace(s[:len(s)-1])
	case strings.HasSuffix(strings.ToLower(s), "g"):
		mult = 1 << 30
		s = strings.TrimSpace(s[:len(s)-1])
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, strconv.ErrRange
	}
	if mult > 1 && n > (1<<63-1)/mult {
		return 0, strconv.ErrRange
	}
	return n * mult, nil
}

func envBool(name string) bool {
	v := strings.TrimSpace(os.Getenv(name))
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
