// SPDX-License-Identifier: LicenseRef-Reticulum
package libmf

import (
	"errors"
	"fmt"
	"sync"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/mf"
)

const (
	OK            = 0
	ErrInvalidArg = 1
	ErrInternal   = 6
	ErrTruncated  = 8
)

var (
	errInvalidArg = errors.New("invalid argument")
	lastErrMu     sync.RWMutex
	lastErr       string
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
	if errors.Is(err, errInvalidArg) {
		return ErrInvalidArg
	}
	return ErrInternal
}

func LastError() string {
	lastErrMu.RLock()
	defer lastErrMu.RUnlock()
	return lastErr
}

func Pack(sender []byte, text string) ([]byte, int) {
	msg, err := mf.NewMessage(sender, text)
	if err != nil {
		return nil, setLastError(mapErr(err))
	}
	data, err := msg.Pack()
	if err != nil {
		return nil, setLastError(mapErr(err))
	}
	return data, OK
}

func Unpack(data []byte) ([]byte, string, int) {
	msg, err := mf.Unpack(data)
	if err != nil {
		return nil, "", setLastError(mapErr(err))
	}
	return msg.SenderHash, msg.Text, OK
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, mf.ErrInvalidHashLength), errors.Is(err, mf.ErrMessageTooShort), errors.Is(err, mf.ErrMessageTooLong):
		return fmt.Errorf("%w: %v", errInvalidArg, err)
	default:
		return err
	}
}
