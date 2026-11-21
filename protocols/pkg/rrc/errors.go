// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import "errors"

var (
	ErrInvalidEnvelope  = errors.New("rrc: invalid envelope")
	ErrEnvelopeTooLarge = errors.New("rrc: envelope exceeds size limit")
	ErrWrongVersion     = errors.New("rrc: unsupported protocol version")
	ErrMissingField     = errors.New("rrc: missing required envelope field")
	ErrBadFieldLength   = errors.New("rrc: fixed field has wrong length")
	ErrNotWelcome       = errors.New("rrc: session not welcomed")
	ErrNotMember        = errors.New("rrc: not a member of room")
	ErrSessionClosed    = errors.New("rrc: session closed")
	ErrLinkInactive     = errors.New("rrc: link not active")
	ErrRateLimited      = errors.New("rrc: rate limit exceeded")
	ErrRoomLimit        = errors.New("rrc: room limit exceeded")
	ErrNickTooLong      = errors.New("rrc: nickname too long")
	ErrRoomNameTooLong  = errors.New("rrc: room name too long")
	ErrBodyTooLarge     = errors.New("rrc: message body too large")
	ErrUnexpectedType   = errors.New("rrc: unexpected message type for session state")
	ErrNilArgument      = errors.New("rrc: nil argument")
	ErrDialTimeout      = errors.New("rrc: dial timeout")
	ErrWelcomeTimeout   = errors.New("rrc: welcome timeout")
	ErrResourceDisabled = errors.New("rrc: resource transfer disabled")
	ErrInvalidHash      = errors.New("rrc: invalid destination hash")
	ErrHub              = errors.New("rrc: hub error")
)
