// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc

import (
	"context"
	"errors"
	"sync"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/rrc"
)

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
	errInvalidHandle = errors.New("invalid handle")
	errInvalidArg    = errors.New("invalid argument")
	errNotFound      = errors.New("not found")
	errState         = errors.New("invalid state")
	errIO            = errors.New("io error")
	errInternal      = errors.New("internal error")
	errTimeout       = errors.New("timeout")

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
	case errors.Is(err, errTimeout), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return ErrTimeout
	case errors.Is(err, errIO):
		return ErrIO
	case errors.Is(err, rrc.ErrDialTimeout), errors.Is(err, rrc.ErrWelcomeTimeout):
		return ErrTimeout
	case errors.Is(err, rrc.ErrNilArgument), errors.Is(err, rrc.ErrBadFieldLength), errors.Is(err, rrc.ErrWrongVersion), errors.Is(err, rrc.ErrInvalidHash):
		return ErrInvalidArg
	case errors.Is(err, errInternal):
		return ErrInternal
	default:
		return ErrInternal
	}
}

func LastError() string {
	lastErrMu.RLock()
	defer lastErrMu.RUnlock()
	return lastErr
}
