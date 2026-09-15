// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package i2p

import "errors"

var (
	ErrSAMOffline      = errors.New("i2p: SAM API offline")
	ErrInvalidResponse = errors.New("i2p: invalid SAM response")
	ErrTunnelSetup     = errors.New("i2p: tunnel setup failed")
	ErrTunnelTimeout   = errors.New("i2p: tunnel setup timed out")
)

// SAMError is a typed SAM RESULT failure, optionally with MESSAGE text.
type SAMError struct {
	Code    string
	Message string
}

func (e *SAMError) Error() string {
	if e == nil {
		return "i2p: SAM error"
	}
	if e.Message != "" {
		return "i2p: SAM " + e.Code + " " + e.Message
	}
	return "i2p: SAM " + e.Code
}

func samErrorFromResult(code, message string) error {
	e := &SAMError{Code: code, Message: message}
	switch code {
	case "CANT_REACH_PEER",
		"DUPLICATED_DEST",
		"DUPLICATED_ID",
		"I2P_ERROR",
		"INVALID_ID",
		"INVALID_KEY",
		"KEY_NOT_FOUND",
		"PEER_NOT_FOUND",
		"TIMEOUT":
		return e
	default:
		return e
	}
}
