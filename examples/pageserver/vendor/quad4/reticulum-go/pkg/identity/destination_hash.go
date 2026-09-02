// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"crypto/sha256"
	"errors"
	"strings"
)

// ExpandAppName builds the dotted destination name used in hashing.
func ExpandAppName(appName string, aspects ...string) string {
	if len(aspects) == 0 {
		return appName
	}
	var name strings.Builder
	name.Grow(len(appName) + len(aspects)*8)
	name.WriteString(appName)
	for _, aspect := range aspects {
		name.WriteByte('.')
		name.WriteString(aspect)
	}
	return name.String()
}

// ParseDestinationName splits a dotted destination name into app name and aspects.
func ParseDestinationName(full string) (appName string, aspects []string, err error) {
	full = strings.TrimSpace(full)
	if full == "" {
		return "", nil, errors.New("empty destination name")
	}
	parts := strings.Split(full, ".")
	if len(parts) < 1 || parts[0] == "" {
		return "", nil, errors.New("invalid destination name")
	}
	if len(parts) == 1 {
		return parts[0], nil, nil
	}
	return parts[0], parts[1:], nil
}

// DestinationHash computes a 16-byte destination hash from an identity and
// app name aspects.
func DestinationHash(id *Identity, appName string, aspects ...string) []byte {
	var idHash []byte
	if id != nil {
		idHash = TruncatedHash(id.GetPublicKey())
	}
	return HashFromIdentityHash(idHash, appName, aspects...)
}

// HashFromIdentityHash computes a destination hash from a 16-byte identity
// hash and app name aspects. Python Destination.hash accepts either an
// Identity or a truncated identity hash.
func HashFromIdentityHash(identityHash []byte, appName string, aspects ...string) []byte {
	nameHashLen := NameHashLength / 8
	outLen := TruncatedHashLength / 8
	nameHashFull := sha256.Sum256([]byte(ExpandAppName(appName, aspects...)))
	nameHash := nameHashFull[:nameHashLen]

	combined := make([]byte, 0, nameHashLen+outLen)
	combined = append(combined, nameHash...)
	if len(identityHash) > 0 {
		n := min(len(identityHash), outLen)
		combined = append(combined, identityHash[:n]...)
	}
	finalHashFull := sha256.Sum256(combined)
	out := make([]byte, outLen)
	copy(out, finalHashFull[:outLen])
	return out
}

// HashFromNameAndIdentity hashes a dotted name such as
// rnstransport.remote.management with a truncated identity hash, matching
// Python Destination.hash_from_name_and_identity.
func HashFromNameAndIdentity(fullName string, identityHash []byte) []byte {
	app, aspects, err := ParseDestinationName(fullName)
	if err != nil {
		return HashFromIdentityHash(identityHash, fullName)
	}
	return HashFromIdentityHash(identityHash, app, aspects...)
}
