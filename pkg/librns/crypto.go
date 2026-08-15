// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"fmt"

	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/rnsutil"
)

// IdentitySign signs data with the identity Ed25519 key.
func IdentitySign(identityHandle uint64, data []byte) ([]byte, int) {
	rec, err := identityByHandle(identityHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	sig, err := rec.identity.Sign(data)
	if err != nil {
		return nil, setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	return sig, OK
}

// IdentityVerify verifies an Ed25519 signature against data.
func IdentityVerify(identityHandle uint64, data, signature []byte) int {
	rec, err := identityByHandle(identityHandle)
	if err != nil {
		return setLastError(err)
	}
	if !rec.identity.Verify(data, signature) {
		return setLastError(fmt.Errorf("%w: signature invalid", errInvalidArg))
	}
	return OK
}

// IdentityPublicKey returns the 64-byte combined public key.
func IdentityPublicKey(identityHandle uint64) ([]byte, int) {
	rec, err := identityByHandle(identityHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	pub := rec.identity.GetPublicKey()
	if len(pub) == 0 {
		return nil, setLastError(fmt.Errorf("%w: empty public key", errInternal))
	}
	return append([]byte(nil), pub...), OK
}

// IdentityFromPublicKey creates a verify-only identity from a 64-byte public key.
func IdentityFromPublicKey(pub []byte) (uint64, int) {
	id := identity.FromPublicKey(pub)
	if id == nil {
		return 0, setLastError(fmt.Errorf("%w: invalid public key", errInvalidArg))
	}
	runtimeMu.Lock()
	handle := handles.insert(kindIdentity, &identityRecord{identity: id})
	runtimeMu.Unlock()
	return handle, OK
}

// IdentityHashBytes returns the 16-byte truncated identity hash.
func IdentityHashBytes(identityHandle uint64) ([]byte, int) {
	rec, err := identityByHandle(identityHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	return append([]byte(nil), rec.identity.Hash()...), OK
}

// RSGCreate builds a detached (.rsg) or embedded (.rsm) signature blob.
func RSGCreate(identityHandle uint64, message []byte, embed bool) ([]byte, int) {
	rec, err := identityByHandle(identityHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	blob, err := rnsutil.CreateRSG(rec.identity, message, embed, nil)
	if err != nil {
		return nil, setLastError(fmt.Errorf("%w: %v", errInternal, err))
	}
	return blob, OK
}

// RSGValidate validates an .rsg/.rsm blob against message bytes.
// requiredSignerHash may be nil or a 16-byte identity hash.
func RSGValidate(rsg, message, requiredSignerHash []byte) int {
	var required any
	if len(requiredSignerHash) > 0 {
		required = requiredSignerHash
	}
	res, err := rnsutil.ValidateRSG(rsg, message, required)
	if err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInvalidArg, err))
	}
	if !res.Valid {
		return setLastError(fmt.Errorf("%w: rsg invalid", errInvalidArg))
	}
	return OK
}

// RSGSignFile signs file contents into a detached .rsg blob.
func RSGSignFile(identityHandle uint64, path string) ([]byte, int) {
	if err := validatePath(path); err != nil {
		return nil, setLastError(err)
	}
	rec, err := identityByHandle(identityHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	blob, err := rnsutil.SignFileRSG(rec.identity, path)
	if err != nil {
		return nil, setLastError(fmt.Errorf("%w: %v", errIO, err))
	}
	return blob, OK
}

// RSGVerifyFile verifies a file against an .rsg blob.
func RSGVerifyFile(rsg []byte, path string, requiredSignerHash []byte) int {
	if err := validatePath(path); err != nil {
		return setLastError(err)
	}
	var required any
	if len(requiredSignerHash) > 0 {
		required = requiredSignerHash
	}
	res, err := rnsutil.VerifyFileRSG(rsg, path, required)
	if err != nil {
		return setLastError(fmt.Errorf("%w: %v", errInvalidArg, err))
	}
	if !res.Valid {
		return setLastError(fmt.Errorf("%w: rsg invalid", errInvalidArg))
	}
	return OK
}

// RSMVerify validates an embedded .rsm and returns the message bytes.
func RSMVerify(rsm, requiredSignerHash []byte) ([]byte, int) {
	var required any
	if len(requiredSignerHash) > 0 {
		required = requiredSignerHash
	}
	res, text, err := rnsutil.VerifyRSM(rsm, required)
	if err != nil {
		return nil, setLastError(fmt.Errorf("%w: %v", errInvalidArg, err))
	}
	if !res.Valid {
		return nil, setLastError(fmt.Errorf("%w: rsm invalid", errInvalidArg))
	}
	return []byte(text), OK
}
