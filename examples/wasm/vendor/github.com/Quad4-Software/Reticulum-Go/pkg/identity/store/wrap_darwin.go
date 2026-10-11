// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build darwin

package store

func defaultWrapCandidates() []wrapCandidate {
	return []wrapCandidate{
		{"keychain", func() (Backend, error) { return NewKeychainBackend() }},
	}
}
