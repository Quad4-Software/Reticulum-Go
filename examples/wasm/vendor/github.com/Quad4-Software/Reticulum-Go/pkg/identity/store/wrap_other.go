// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !linux && !darwin && !windows

package store

// BSDs and other platforms have no standard user credential store. RNE1
// passphrase mode still works there via env, fd, or interactive prompt.
func defaultWrapCandidates() []wrapCandidate {
	return nil
}
