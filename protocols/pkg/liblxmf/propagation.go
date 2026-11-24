// SPDX-License-Identifier: LicenseRef-Reticulum
package liblxmf

import (
	"fmt"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxmf"
)

// MessagePackPropagated packs a message for propagation-node upload.
// recipientHash is the 16-byte LXMF delivery hash.
// recipientPublicKey is 64-byte Ed25519+X25519 material (identity.GetPublicKey / FromPublicKey).
// pnStampCost is the prop-node stamp cost (0 means no PN stamp).
// Returns msgpack outer payload suitable for link SendPacket / SendResource.
func MessagePackPropagated(
	messageHandle, identityHandle uint64,
	recipientHash, recipientPublicKey []byte,
	pnStampCost int,
) ([]byte, int) {
	rec, err := messageByHandle(messageHandle)
	if err != nil {
		return nil, setLastError(err)
	}
	id, code := identityFromHandle(identityHandle)
	if code != OK {
		return nil, code
	}
	if len(recipientHash) != lxmf.DestinationLength {
		return nil, setLastError(fmt.Errorf("%w: recipient hash length", errInvalidArg))
	}
	if len(recipientPublicKey) != identity.KeySize/8 {
		return nil, setLastError(fmt.Errorf("%w: recipient public key length", errInvalidArg))
	}

	remoteIdentity := identity.FromPublicKey(recipientPublicKey)
	if remoteIdentity == nil {
		return nil, setLastError(fmt.Errorf("%w: recipient public key", errInvalidArg))
	}

	recipient, err := destination.FromHash(recipientHash, remoteIdentity, destination.Single, nil)
	if err != nil {
		return nil, setLastError(fmt.Errorf("create recipient destination: %w", err))
	}

	if len(rec.msg.Packed) == 0 {
		if _, err := rec.msg.Pack(id); err != nil {
			return nil, setLastError(err)
		}
	}

	rec.msg.PropagationStamp = nil
	rec.msg.PropagationPacked = nil
	rec.msg.TransientID = nil
	if err := rec.msg.PackPropagated(recipient, pnStampCost); err != nil {
		return nil, setLastError(err)
	}
	return append([]byte(nil), rec.msg.PropagationPacked...), OK
}
