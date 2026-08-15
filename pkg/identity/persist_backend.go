// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"fmt"

	"quad4/reticulum-go/pkg/identity/store"
)

// SetIdentityBackend selects file or secretservice persistence for ToFile.
func SetIdentityBackend(name string) error {
	return store.SetBackendName(name)
}

// IdentityBackend returns the configured persistence backend name.
func IdentityBackend() string {
	return store.BackendName()
}

// ApplyIdentityBackendFromConfig sets the backend from config (empty => file).
func ApplyIdentityBackendFromConfig(name string) error {
	if name == "" {
		name = store.BackendFile
	}
	if err := store.SetBackendName(name); err != nil {
		return fmt.Errorf("identity_backend: %w", err)
	}
	return nil
}
