// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package store

func defaultWrapCandidates() []wrapCandidate {
	return []wrapCandidate{
		{"keyring", func() (Backend, error) { return NewKeyringBackend() }},
		{"secretservice", func() (Backend, error) { return NewSecretServiceBackend() }},
	}
}
