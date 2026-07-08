// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package common

import (
	"os"
	"testing"
)

func TestApplyPersistenceEnv(t *testing.T) {
	t.Setenv("RETICULUM_IN_MEMORY_PATH_TABLE", "1")
	t.Setenv("RETICULUM_IN_MEMORY_KNOWN_DESTINATIONS", "")
	cfg := NewReticulumConfig()
	cfg.ApplyPersistenceEnv()
	if !cfg.InMemoryPathTable {
		t.Fatal("expected in-memory path table from env")
	}
	if cfg.InMemoryKnownDestinations {
		t.Fatal("known destinations should stay on disk")
	}

	t.Setenv("RETICULUM_IN_MEMORY_STORAGE", "true")
	cfg = NewReticulumConfig()
	cfg.ApplyPersistenceEnv()
	if !cfg.InMemoryPathTable || !cfg.InMemoryKnownDestinations {
		t.Fatal("RETICULUM_IN_MEMORY_STORAGE should force both tables in-memory")
	}
}

func TestEnvBool(t *testing.T) {
	os.Unsetenv("RETICULUM_TEST_BOOL")
	if envBool("RETICULUM_TEST_BOOL") {
		t.Fatal("unset env should be false")
	}
	t.Setenv("RETICULUM_TEST_BOOL", "yes")
	if !envBool("RETICULUM_TEST_BOOL") {
		t.Fatal("yes should be true")
	}
}
