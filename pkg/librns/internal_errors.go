// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import "errors"

var (
	errInvalidHandle = errors.New("invalid handle")
	errInvalidArg    = errors.New("invalid argument")
	errNotFound      = errors.New("not found")
	errState         = errors.New("invalid state")
	errTimeout       = errors.New("timeout")
	errIO            = errors.New("io error")
	errInternal      = errors.New("internal error")
)
