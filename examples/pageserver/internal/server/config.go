// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package server

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
)

// ClampDisplayName trims name, applies fallback when empty, and truncates to
// MaxNodeNameBytes for announce app_data.
func ClampDisplayName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fallback
	}
	if len(name) > MaxNodeNameBytes {
		debug.Log(debug.DebugCritical,
			"node name exceeds max length, truncating",
			"max_bytes", MaxNodeNameBytes,
			"got_bytes", len(name),
		)
		name = name[:MaxNodeNameBytes]
	}
	return name
}

// ResolveIdentityPath returns override when set, otherwise the default identity
// file under the current user's home.
func ResolveIdentityPath(override string) string {
	if override != "" {
		return override
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".reticulum-go", "storage", "identity")
	}
	return filepath.Join(homeDir, ".reticulum-go", "storage", "identity")
}
