// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"time"
)

type PrepareFreshPathReturn string

const (
	PrepareFreshReusedValidPath      PrepareFreshPathReturn = "reused_valid_path"
	PrepareFreshPathRefreshRequested PrepareFreshPathReturn = "path_refresh_requested"
	PrepareFreshNewPathRequested     PrepareFreshPathReturn = "new_path_requested"
	PrepareFreshInvalidDestination   PrepareFreshPathReturn = "invalid_destination"
)

// PrepareFreshPathRequest ensures a path discovery packet is sent when the cached
// route is missing, marked unresponsive, or expired (same TTL rules as HasPath).
func (t *Transport) PrepareFreshPathRequest(destinationHash []byte) PrepareFreshPathReturn {
	if t == nil || len(destinationHash) != 16 {
		return PrepareFreshInvalidDestination
	}
	key := string(destinationHash)
	ttl := time.Duration(PathRequestTTL) * time.Second

	t.mutex.Lock()
	path, pathExists := t.paths[key]
	valid := pathExists && path != nil && time.Since(path.LastUpdated) <= ttl
	unresponsive := false
	if st, ok := t.pathStates[key]; ok && st == StateUnresponsive {
		unresponsive = true
	}

	if valid && !unresponsive {
		t.mutex.Unlock()
		return PrepareFreshReusedValidPath
	}

	hadPath := pathExists && path != nil
	if hadPath {
		delete(t.paths, key)
		delete(t.pathStates, key)
	}
	delete(t.lastPathRequest, key)
	t.mutex.Unlock()

	_ = t.RequestPath(destinationHash, "", nil, false)
	if hadPath {
		return PrepareFreshPathRefreshRequested
	}
	return PrepareFreshNewPathRequested
}

// NudgePathRequest clears the per-destination throttle used by RequestPath when tag
// is nil, then sends one path request. Use while polling for HasPath at intervals
// of at least PathRequestMI so discovery packets actually emit on each nudge.
func (t *Transport) NudgePathRequest(destinationHash []byte) error {
	if t == nil || len(destinationHash) != 16 {
		return nil
	}
	t.mutex.Lock()
	delete(t.lastPathRequest, string(destinationHash))
	t.mutex.Unlock()
	return t.RequestPath(destinationHash, "", nil, false)
}

// ExpirePath drops the cached path entry (and path state / throttle hints) for this
// destination so HasPath returns false until the next successful path update.
// It does not send a packet; pair with RequestPath or PrepareFreshPathRequest.
func (t *Transport) ExpirePath(destinationHash []byte) {
	if t == nil || len(destinationHash) != 16 {
		return
	}
	key := string(destinationHash)
	t.mutex.Lock()
	defer t.mutex.Unlock()
	delete(t.paths, key)
	delete(t.pathStates, key)
	delete(t.lastPathRequest, key)
}
