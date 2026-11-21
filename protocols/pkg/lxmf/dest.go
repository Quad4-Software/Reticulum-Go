// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"crypto/sha256"
	"strings"
)

const (
	AspectDelivery    = "delivery"
	AspectPropagation = "propagation"
	appHashPrefixLen  = 10
)

func DestHash(identityHash []byte) []byte {
	return namedDestHash(identityHash, AppName, AspectDelivery)
}

func namedDestHash(identityHash []byte, appName string, aspects ...string) []byte {
	if len(identityHash) != DestinationLength {
		return nil
	}
	var name strings.Builder
	name.Grow(len(appName) + 16)
	name.WriteString(appName)
	for _, a := range aspects {
		name.WriteByte('.')
		name.WriteString(a)
	}
	sum := sha256.Sum256([]byte(name.String()))
	combined := make([]byte, appHashPrefixLen+len(identityHash))
	copy(combined, sum[:appHashPrefixLen])
	copy(combined[appHashPrefixLen:], identityHash)
	final := sha256.Sum256(combined)
	return final[:DestinationLength:DestinationLength]
}

type destID [DestinationLength]byte

func destIDFrom(h []byte) (destID, bool) {
	var k destID
	if len(h) != DestinationLength {
		return k, false
	}
	copy(k[:], h)
	return k, true
}
