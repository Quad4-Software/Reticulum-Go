// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"errors"
	"sync"
)

// Error codes returned across the librns API.
const (
	OK               = 0
	ErrInvalidArg    = 1
	ErrInvalidHandle = 2
	ErrNotFound      = 3
	ErrState         = 4
	ErrIO            = 5
	ErrInternal      = 6
	ErrTimeout       = 7
	ErrTruncated     = 8
)

var (
	lastErrMu sync.RWMutex
	lastErr   string
)

func setLastError(err error) int {
	if err == nil {
		lastErrMu.Lock()
		lastErr = ""
		lastErrMu.Unlock()
		return OK
	}
	lastErrMu.Lock()
	lastErr = err.Error()
	lastErrMu.Unlock()
	return mapError(err)
}

func mapError(err error) int {
	if err == nil {
		return OK
	}
	switch {
	case errors.Is(err, errInvalidHandle):
		return ErrInvalidHandle
	case errors.Is(err, errInvalidArg):
		return ErrInvalidArg
	case errors.Is(err, errNotFound):
		return ErrNotFound
	case errors.Is(err, errState):
		return ErrState
	case errors.Is(err, errTimeout):
		return ErrTimeout
	case errors.Is(err, errIO):
		return ErrIO
	case errors.Is(err, errInternal):
		return ErrInternal
	default:
		return ErrInternal
	}
}

// LastError returns the message for the most recent failing librns call.
func LastError() string {
	lastErrMu.RLock()
	defer lastErrMu.RUnlock()
	return lastErr
}
