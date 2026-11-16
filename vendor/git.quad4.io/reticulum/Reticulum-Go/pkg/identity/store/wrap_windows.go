// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build windows

package store

func defaultWrapCandidates() []wrapCandidate {
	return []wrapCandidate{
		{"dpapi", func() (Backend, error) { return NewDPAPIBackend() }},
	}
}
