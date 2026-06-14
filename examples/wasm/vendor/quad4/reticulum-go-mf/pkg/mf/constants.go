package mf

import "errors"

const (
	// SenderHashLength is the sender hash size in bytes.
	SenderHashLength = 16
	// MaxMessageSize is max text length (MTU 500 − header 64 − SenderHashLength 16).
	MaxMessageSize = 420

	testHashHex    = "0123456789abcdef0123456789abcdef"
	errFmtExpected = "%w: expected %d, got %d"
)

var (
	// ErrInvalidHashLength means the hash length is not SenderHashLength.
	ErrInvalidHashLength = errors.New("invalid hash length")
	// ErrMessageTooShort means the buffer is shorter than SenderHashLength.
	ErrMessageTooShort = errors.New("message too short")
	// ErrMessageTooLong means text exceeds MaxMessageSize.
	ErrMessageTooLong = errors.New("message text too long")
)
