// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !unix || haiku

package debug

import (
	"errors"
	"io"
)

func openSyslogWriter(tag string) (io.Writer, error) {
	return nil, errors.New("syslog is not supported on this platform")
}

func openJournaldWriter(tag string) (io.Writer, error) {
	return nil, errors.New("journald is not supported on this platform")
}
