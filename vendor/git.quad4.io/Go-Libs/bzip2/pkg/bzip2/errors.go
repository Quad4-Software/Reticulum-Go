// SPDX-License-Identifier: 0BSD
// Copyright (c)2026 Quad4.io

package bzip2

import "errors"

var (
	// ErrClosed is returned when Write or Close is called after Close has completed successfully.
	ErrClosed = errors.New("bzip2: writer closed")
	// ErrLevelRange is returned when NewWriter is called with a level outside 1–9.
	ErrLevelRange = errors.New("bzip2: level must be between 1 and 9")
	// ErrNilWriter is returned when NewWriter is called with a nil destination writer.
	ErrNilWriter = errors.New("bzip2: nil io.Writer")
)
