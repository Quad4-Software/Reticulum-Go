// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build unix && !linux && !haiku

package debug

import (
	"fmt"
	"io"
	"log/syslog"
)

func openSyslogWriter(tag string) (io.Writer, error) {
	if tag == "" {
		tag = "reticulum-go"
	}
	w, err := syslog.New(syslog.LOG_DAEMON|syslog.LOG_INFO, tag)
	if err != nil {
		return nil, fmt.Errorf("syslog: %w", err)
	}
	return w, nil
}

func openJournaldWriter(tag string) (io.Writer, error) {
	return openSyslogWriter(tag)
}
