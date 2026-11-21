// SPDX-License-Identifier: LicenseRef-Reticulum

package session

import "errors"

var (
	ErrNoTransport = errors.New("transport required")
	ErrNoIdentity  = errors.New("identity required")
	ErrNoHost      = errors.New("session has no host pcm device")
	ErrNoCall      = errors.New("no active call")
	ErrInvalidHash = errors.New("invalid destination hash")
	ErrRecall      = errors.New("could not recall identity")
	ErrAttach      = errors.New("pcm stream")
	ErrNoStream    = errors.New("missing pcm stream")
	ErrFingerprint = errors.New("caller hash mismatch")
	ErrUnknownName = errors.New("unknown profile or mode name")
	ErrPCMDropped  = errors.New("host pcm capture dropped")
)
