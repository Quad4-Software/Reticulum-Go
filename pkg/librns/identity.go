// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"fmt"

	"quad4/reticulum-go/pkg/identity"
)

// IdentityGenerate creates a new software identity handle.
func IdentityGenerate() (uint64, int) {
	id, err := identity.New()
	if err != nil {
		return 0, setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	runtimeMu.Lock()
	handle := handles.insert(kindIdentity, &identityRecord{identity: id})
	runtimeMu.Unlock()
	return handle, OK
}

// IdentityLoad loads a software or hardware-bound identity from path.
func IdentityLoad(path string) (uint64, int) {
	if err := validatePath(path); err != nil {
		return 0, setLastError(err)
	}
	id, err := identity.LoadIdentityFile(path, nil)
	if err != nil {
		return 0, setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	runtimeMu.Lock()
	handle := handles.insert(kindIdentity, &identityRecord{identity: id})
	runtimeMu.Unlock()
	return handle, OK
}

// IdentityDestroy releases an identity handle.
func IdentityDestroy(identityHandle uint64) int {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if !handles.delete(identityHandle) {
		return setLastError(errInvalidHandle)
	}
	return OK
}

// IdentityHashHex returns the truncated identity hash as lowercase hex.
func IdentityHashHex(identityHandle uint64) (string, int) {
	rec, err := identityByHandle(identityHandle)
	if err != nil {
		return "", setLastError(err)
	}
	return rec.identity.GetHexHash(), OK
}
