// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc

import (
	"fmt"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

type identityRecord struct {
	identity *identity.Identity
}

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

func IdentityLoad(path string) (uint64, int) {
	if err := validatePath(path); err != nil {
		return 0, setLastError(err)
	}
	id, err := identity.FromFile(path)
	if err != nil {
		return 0, setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	runtimeMu.Lock()
	handle := handles.insert(kindIdentity, &identityRecord{identity: id})
	runtimeMu.Unlock()
	return handle, OK
}

func IdentitySave(handle uint64, path string) int {
	rec, err := identityByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	if err := validatePath(path); err != nil {
		return setLastError(err)
	}
	if err := rec.identity.ToFile(path); err != nil {
		return setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	return OK
}

func IdentityDestroy(handle uint64) int {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if !handles.delete(handle) {
		return setLastError(errInvalidHandle)
	}
	return OK
}

func IdentityHash(handle uint64) ([]byte, int) {
	rec, err := identityByHandle(handle)
	if err != nil {
		return nil, setLastError(err)
	}
	return append([]byte(nil), rec.identity.Hash()...), OK
}

func identityByHandle(id uint64) (*identityRecord, error) {
	ref, err := handles.get(id, kindIdentity)
	if err != nil {
		return nil, err
	}
	return ref.(*identityRecord), nil
}
