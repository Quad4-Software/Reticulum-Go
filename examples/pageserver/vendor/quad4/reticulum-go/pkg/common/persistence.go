// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package common

import (
	"os"
	"strings"
)

// ApplyPersistenceEnv overrides persistence-related config from environment
// variables. Set RETICULUM_IN_MEMORY_PATH_TABLE=1 or
// RETICULUM_IN_MEMORY_KNOWN_DESTINATIONS=1 to force in-memory tables.
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
		c.InMemoryPathTable = true
		c.InMemoryKnownDestinations = true
	}
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
