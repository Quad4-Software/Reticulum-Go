// SPDX-License-Identifier: LicenseRef-Reticulum
package liblxmf

import (
	"fmt"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxmf"
)

type identityRecord struct {
	identity *identity.Identity
}

func IdentityGenerate() (uint64, int) {
	id, err := identity.New()
	if err != nil {
		return 0, setLastError(err)
	}
	return handles.insert(kindIdentity, &identityRecord{identity: id}), OK
}

func IdentityLoad(path string) (uint64, int) {
	if path == "" {
		return 0, setLastError(errInvalidArg)
	}
	for i := 0; i < len(path); i++ {
		if path[i] == 0 {
			return 0, setLastError(errInvalidArg)
		}
	}
	id, err := identity.LoadIdentityFile(path, nil)
	if err != nil {
		return 0, setLastError(err)
	}
	return handles.insert(kindIdentity, &identityRecord{identity: id}), OK
}

func IdentitySave(handle uint64, path string) int {
	if path == "" {
		return setLastError(errInvalidArg)
	}
	for i := 0; i < len(path); i++ {
		if path[i] == 0 {
			return setLastError(errInvalidArg)
		}
	}
	rec, err := identityByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	if err := rec.identity.ToFile(path); err != nil {
		return setLastError(err)
	}
	return OK
}

func IdentityDestroy(handle uint64) int {
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

func IdentityPublicKey(handle uint64) ([]byte, int) {
	rec, err := identityByHandle(handle)
	if err != nil {
		return nil, setLastError(err)
	}
	return append([]byte(nil), rec.identity.GetPublicKey()...), OK
}

func IdentityDeliveryHash(handle uint64) ([]byte, int) {
	rec, err := identityByHandle(handle)
	if err != nil {
		return nil, setLastError(err)
	}
	dest, err := destination.New(rec.identity, destination.Out, destination.Single, lxmf.AppName, nil, "delivery")
	if err != nil {
		return nil, setLastError(err)
	}
	return append([]byte(nil), dest.GetHash()...), OK
}

func IdentityRegisterRecall(handle uint64) int {
	rec, err := identityByHandle(handle)
	if err != nil {
		return setLastError(err)
	}
	return IdentityRegisterRecallBytes(rec.identity)
}

func IdentityRegisterRecallBytes(id *identity.Identity) int {
	if id == nil {
		return setLastError(fmt.Errorf("%w: identity nil", errInvalidArg))
	}
	dest, err := destination.New(id, destination.Out, destination.Single, lxmf.AppName, nil, "delivery")
	if err != nil {
		return setLastError(err)
	}
	identity.Remember(nil, dest.GetHash(), id.GetPublicKey(), nil)
	return OK
}

func IdentityRegisterRecallSource(sourceHash, publicKey []byte) int {
	if len(sourceHash) != lxmf.DestinationLength || len(publicKey) == 0 {
		return setLastError(fmt.Errorf("%w: recall source", errInvalidArg))
	}
	identity.Remember(nil, append([]byte(nil), sourceHash...), append([]byte(nil), publicKey...), nil)
	return OK
}

func identityByHandle(id uint64) (*identityRecord, error) {
	ref, err := handles.get(id, kindIdentity)
	if err != nil {
		return nil, err
	}
	return ref.(*identityRecord), nil
}

func identityFromHandle(id uint64) (*identity.Identity, int) {
	rec, err := identityByHandle(id)
	if err != nil {
		return nil, setLastError(err)
	}
	return rec.identity, OK
}
