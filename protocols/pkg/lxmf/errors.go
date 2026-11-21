// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import "errors"

// Errors returned by this package.
var (
	// ErrInvalidHashLength means a hash length is not DestinationLength.
	ErrInvalidHashLength = errors.New("invalid hash length")
	// ErrMessageTooShort means the buffer is shorter than minimum LXMF overhead.
	ErrMessageTooShort = errors.New("message too short")
	// ErrInvalidPayload means msgpack decoding did not match the expected shape.
	ErrInvalidPayload = errors.New("invalid payload")
	// ErrSignatureInvalid means the Ed25519 signature does not verify.
	ErrSignatureInvalid = errors.New("signature invalid")
	// ErrSourceUnknown means the source identity could not be resolved.
	ErrSourceUnknown = errors.New("source identity unknown")
	// ErrDestinationUnknown means the destination identity could not be resolved.
	ErrDestinationUnknown = errors.New("destination identity unknown")
	// ErrAlreadyPacked means Pack was called on an already packed message.
	ErrAlreadyPacked = errors.New("message already packed")
	// ErrContentTooLarge means the payload exceeds the chosen delivery limit.
	ErrContentTooLarge = errors.New("content exceeds delivery method limit")
	// ErrUnsupportedMethod means the delivery method is not implemented here.
	ErrUnsupportedMethod = errors.New("unsupported delivery method")
	// ErrNoPropagationNode means no propagation node was discovered or reachable in time.
	ErrNoPropagationNode = errors.New("no propagation node available")
)
