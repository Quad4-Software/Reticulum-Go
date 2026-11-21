// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc

import (
	"fmt"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

func IdentitySeedDestination(identityHandle uint64, destHash []byte) int {
	if len(destHash) != rrc.IdentityLength {
		return setLastError(fmt.Errorf("%w: destination hash", errInvalidArg))
	}
	idRec, err := identityByHandle(identityHandle)
	if err != nil {
		return setLastError(err)
	}
	identity.Remember(nil, destHash, idRec.identity.GetPublicKey(), nil)
	return OK
}
