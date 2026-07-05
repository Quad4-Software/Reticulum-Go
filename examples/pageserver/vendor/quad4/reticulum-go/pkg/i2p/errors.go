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

type SAMError struct {
	Code string
}

func (e *SAMError) Error() string {
	return "i2p: SAM " + e.Code
}

func samErrorFromResult(code string) error {
	switch code {
	case "CANT_REACH_PEER":
		return &SAMError{Code: "CANT_REACH_PEER"}
	case "DUPLICATED_DEST":
		return &SAMError{Code: "DUPLICATED_DEST"}
	case "DUPLICATED_ID":
		return &SAMError{Code: "DUPLICATED_ID"}
	case "I2P_ERROR":
		return &SAMError{Code: "I2P_ERROR"}
	case "INVALID_ID":
		return &SAMError{Code: "INVALID_ID"}
	case "INVALID_KEY":
		return &SAMError{Code: "INVALID_KEY"}
	case "KEY_NOT_FOUND":
		return &SAMError{Code: "KEY_NOT_FOUND"}
	case "PEER_NOT_FOUND":
		return &SAMError{Code: "PEER_NOT_FOUND"}
	case "TIMEOUT":
		return &SAMError{Code: "TIMEOUT"}
	default:
		return &SAMError{Code: code}
	}
}
