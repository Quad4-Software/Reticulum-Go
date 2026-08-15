// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"errors"
	"testing"
)

func TestApplyPersistenceEnv(t *testing.T) {
	t.Setenv("RETICULUM_IN_MEMORY_PATH_TABLE", "1")
	t.Setenv("RETICULUM_IN_MEMORY_KNOWN_DESTINATIONS", "")
	t.Setenv("RETICULUM_IN_MEMORY_STORAGE", "")
	t.Setenv("RETICULUM_SOFT_MEMORY_LIMIT", "")
	t.Setenv("RETICULUM_MAX_PACKET_HASHLIST", "")
	t.Setenv("RETICULUM_MAX_IN_MEMORY_PATHS", "")
	t.Setenv("RETICULUM_MAX_IN_MEMORY_KNOWN_DESTINATIONS", "")
	cfg := NewReticulumConfig()
	cfg.ApplyPersistenceEnv()
	if !cfg.InMemoryPathTable {
		t.Fatal("expected in-memory path table from env")
	}
	if cfg.InMemoryKnownDestinations {
		t.Fatal("known destinations should stay on disk")
	}
	if cfg.InMemoryStorage {
		t.Fatal("full in-memory storage should stay off")
	}

	t.Setenv("RETICULUM_IN_MEMORY_STORAGE", "true")
	cfg = NewReticulumConfig()
	cfg.ApplyPersistenceEnv()
	if !cfg.InMemoryStorage || !cfg.InMemoryPathTable || !cfg.InMemoryKnownDestinations {
		t.Fatal("RETICULUM_IN_MEMORY_STORAGE should force full ephemeral mode")
	}

	t.Setenv("RETICULUM_SOFT_MEMORY_LIMIT", "64M")
	cfg = NewReticulumConfig()
	cfg.ApplyPersistenceEnv()
	if cfg.SoftMemoryLimitBytes != 64<<20 {
		t.Fatalf("soft memory limit = %d, want %d", cfg.SoftMemoryLimitBytes, 64<<20)
	}

	t.Setenv("RETICULUM_MAX_PACKET_HASHLIST", "8192")
	t.Setenv("RETICULUM_MAX_IN_MEMORY_PATHS", "1234")
	t.Setenv("RETICULUM_MAX_IN_MEMORY_KNOWN_DESTINATIONS", "5678")
	cfg = NewReticulumConfig()
	cfg.ApplyPersistenceEnv()
	if cfg.MaxPacketHashlist != 8192 {
		t.Fatalf("MaxPacketHashlist = %d, want 8192", cfg.MaxPacketHashlist)
	}
	if cfg.MaxInMemoryPaths != 1234 {
		t.Fatalf("MaxInMemoryPaths = %d, want 1234", cfg.MaxInMemoryPaths)
	}
	if cfg.MaxInMemoryKnownDestinations != 5678 {
		t.Fatalf("MaxInMemoryKnownDestinations = %d, want 5678", cfg.MaxInMemoryKnownDestinations)
	}
}

func TestEnvBool(t *testing.T) {
	t.Setenv("RETICULUM_TEST_BOOL", "")
	if envBool("RETICULUM_TEST_BOOL") {
		t.Fatal("unset env should be false")
	}
	t.Setenv("RETICULUM_TEST_BOOL", "yes")
	if !envBool("RETICULUM_TEST_BOOL") {
		t.Fatal("yes should be true")
	}
}

func TestUseInMemoryStorage(t *testing.T) {
	t.Setenv("RETICULUM_STORAGE_PATH", "")
	if !(*ReticulumConfig)(nil).UseInMemoryStorage() {
		t.Fatal("nil config should be in-memory")
	}
	cfg := NewReticulumConfig()
	if !cfg.UseInMemoryStorage() {
		t.Fatal("empty ConfigPath should be in-memory")
	}
	cfg.ConfigPath = "/tmp/reticulum-go/config"
	if cfg.UseInMemoryStorage() {
		t.Fatal("ConfigPath without flag should use disk")
	}
	cfg.InMemoryStorage = true
	if !cfg.UseInMemoryStorage() {
		t.Fatal("InMemoryStorage flag must force ephemeral mode")
	}
	cfg.InMemoryStorage = false
	t.Setenv("RETICULUM_STORAGE_PATH", "/tmp/rns-storage")
	cfg.ConfigPath = ""
	if cfg.UseInMemoryStorage() {
		t.Fatal("RETICULUM_STORAGE_PATH should allow disk persistence")
	}
}

func TestParseByteSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1024", 1024},
		{"1K", 1024},
		{"2M", 2 << 20},
		{"1G", 1 << 30},
		{" 8m ", 8 << 20},
	}
	for _, tc := range cases {
		got, err := ParseByteSize(tc.in)
		if err != nil {
			t.Fatalf("ParseByteSize(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseByteSize(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
	if _, err := ParseByteSize(""); err == nil {
		t.Fatal("empty should error")
	}
	if _, err := ParseByteSize("-1"); err == nil {
		t.Fatal("negative should error")
	}
}

func TestMemoryBudgetReserveAndRelease(t *testing.T) {
	b := NewMemoryBudget(100)
	if err := b.TryReserve(60); err != nil {
		t.Fatal(err)
	}
	if err := b.TryReserve(50); !errors.Is(err, ErrMemoryBudgetExceeded) {
		t.Fatalf("expected ErrMemoryBudgetExceeded, got %v", err)
	}
	if b.Used() != 60 {
		t.Fatalf("used = %d, want 60", b.Used())
	}
	b.Release(20)
	if err := b.TryReserve(50); err != nil {
		t.Fatal(err)
	}
	if b.Used() != 90 {
		t.Fatalf("used = %d, want 90", b.Used())
	}
	b.Release(1000)
	if b.Used() != 0 {
		t.Fatalf("used should clamp to 0, got %d", b.Used())
	}
}

func TestMemoryBudgetUnlimited(t *testing.T) {
	b := NewMemoryBudget(0)
	if err := b.TryReserve(1 << 30); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryBudgetSetLimitAndApplySoft(t *testing.T) {
	b := NewMemoryBudget(10)
	if b.Limit() != 10 {
		t.Fatalf("Limit = %d, want 10", b.Limit())
	}
	if err := b.TryReserve(8); err != nil {
		t.Fatal(err)
	}
	b.SetLimit(5)
	if b.Limit() != 5 {
		t.Fatalf("Limit after SetLimit = %d, want 5", b.Limit())
	}
	if err := b.TryReserve(1); !errors.Is(err, ErrMemoryBudgetExceeded) {
		t.Fatalf("after shrinking limit, reserve should fail with ErrMemoryBudgetExceeded, got %v", err)
	}
	b.SetLimit(0)
	if err := b.TryReserve(100); err != nil {
		t.Fatalf("unlimited after SetLimit(0): %v", err)
	}

	prev := ApplySoftMemoryLimit(32 << 20)
	t.Cleanup(func() { ApplySoftMemoryLimit(prev) })
	disabled := ApplySoftMemoryLimit(0)
	if disabled != 32<<20 {
		t.Fatalf("ApplySoftMemoryLimit(0) should restore unlimited and return prior limit, got %d", disabled)
	}
}

func TestEffectiveCaps(t *testing.T) {
	t.Setenv("RETICULUM_STORAGE_PATH", "")
	cfg := &ReticulumConfig{InMemoryStorage: true}
	if cfg.EffectiveMaxInMemoryPaths() != DefaultMaxInMemoryPaths {
		t.Fatal("default path cap")
	}
	cfg.MaxInMemoryPaths = -1
	if cfg.EffectiveMaxInMemoryPaths() != 0 {
		t.Fatal("negative disables path cap")
	}
	cfg.MaxInMemoryPaths = 42
	if cfg.EffectiveMaxInMemoryPaths() != 42 {
		t.Fatal("explicit path cap")
	}
	cfg.InMemoryStorage = false
	cfg.ConfigPath = "/tmp/reticulum-go/config"
	cfg.MaxInMemoryPaths = 0
	if cfg.EffectiveMaxInMemoryPaths() != DefaultMaxInMemoryPaths {
		t.Fatal("disk-backed stacks still get the default path cap")
	}

	cfg = &ReticulumConfig{InMemoryStorage: true}
	if cfg.EffectiveMaxInMemoryKnownDestinations() != DefaultMaxInMemoryKnownDestinations {
		t.Fatal("default known-dest cap")
	}
	if cfg.EffectiveMaxInMemoryResourceBytes() != DefaultMaxInMemoryResourceBytes {
		t.Fatal("default resource budget")
	}
	cfg.MaxInMemoryKnownDestinations = 7
	cfg.MaxInMemoryResourceBytes = 9
	if cfg.EffectiveMaxInMemoryKnownDestinations() != 7 {
		t.Fatal("explicit known-dest cap")
	}
	if cfg.EffectiveMaxInMemoryResourceBytes() != 9 {
		t.Fatal("explicit resource budget")
	}
	cfg.MaxInMemoryKnownDestinations = -1
	cfg.MaxInMemoryResourceBytes = -1
	if cfg.EffectiveMaxInMemoryKnownDestinations() != 0 || cfg.EffectiveMaxInMemoryResourceBytes() != 0 {
		t.Fatal("negative should disable caps")
	}
	cfg.InMemoryStorage = false
	cfg.MaxInMemoryKnownDestinations = 0
	if cfg.EffectiveMaxInMemoryKnownDestinations() != DefaultMaxInMemoryKnownDestinations {
		t.Fatal("disk-backed stacks still get the default known-dest cap")
	}
	if cfg.EffectiveMaxInMemoryResourceBytes() != 0 {
		t.Fatal("resource budget still requires InMemoryStorage")
	}

	cfg = &ReticulumConfig{EnableTransport: true}
	if cfg.EffectiveMaxPacketHashlist() != DefaultMaxPacketHashlist {
		t.Fatal("transport hashlist default")
	}
	cfg.EnableTransport = false
	if cfg.EffectiveMaxPacketHashlist() != DefaultMaxPacketHashlistClient {
		t.Fatal("client hashlist default")
	}
	cfg.MaxPacketHashlist = 4096
	if cfg.EffectiveMaxPacketHashlist() != 4096 {
		t.Fatal("explicit hashlist cap")
	}
	cfg.MaxPacketHashlist = -1
	if cfg.EffectiveMaxPacketHashlist() != DefaultMaxPacketHashlist {
		t.Fatal("negative hashlist forces full transport size")
	}
}
