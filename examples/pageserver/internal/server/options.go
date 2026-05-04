// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package server

import "time"

// Options are runtime paths and intervals for the page node (from CLI flags).
type Options struct {
	PagesDir                string
	FilesDir                string
	PageRefreshInterval     time.Duration
	FileRefreshInterval     time.Duration
	AnnounceIntervalMinutes int
	IdentityFileOverride    string
	NodeDisplayName         string
}
