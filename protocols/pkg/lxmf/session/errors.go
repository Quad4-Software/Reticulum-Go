// SPDX-License-Identifier: LicenseRef-Reticulum

package session

import "errors"

var (
	ErrNoTransport      = errors.New("transport required")
	ErrNoIdentity       = errors.New("identity required")
	ErrInvalidHash      = errors.New("invalid destination hash")
	ErrRecall           = errors.New("could not recall identity")
	ErrFingerprint      = errors.New("peer hash mismatch")
	ErrRateLimited      = errors.New("sender rate limited")
	ErrNotAllowed       = errors.New("sender not allowed")
	ErrUnverified       = errors.New("signature not verified")
	ErrStamp            = errors.New("stamp required")
	ErrNoPropagation    = errors.New("no propagation node")
	ErrRequireStampCost = errors.New("require stamp needs a positive stamp cost")
)
