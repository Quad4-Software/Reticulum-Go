// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !linux

package interfaces

// platformNetwatchSubscribe is a no-op on platforms without an rtnetlink
// watcher. Reconnect backoff still applies. Recovery waits for the
// normal timer instead of an early kick.
func platformNetwatchSubscribe(func()) func() {
	return func() {}
}
