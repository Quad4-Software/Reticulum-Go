// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package debug

import (
	"fmt"
	"io"
	"log/syslog"
	"net"
	"strings"
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

type journalWriter struct {
	conn net.Conn
	tag  string
}

func openJournaldWriter(tag string) (io.Writer, error) {
	if tag == "" {
		tag = "reticulum-go"
	}
	conn, err := net.Dial("unixgram", "/run/systemd/journal/socket")
	if err != nil {
		// Fall back to syslog when journald socket is unavailable.
		return openSyslogWriter(tag)
	}
	return &journalWriter{conn: conn, tag: tag}, nil
}

func (j *journalWriter) Write(p []byte) (int, error) {
	if j == nil || j.conn == nil {
		return 0, io.ErrClosedPipe
	}
	msg := strings.ReplaceAll(strings.TrimRight(string(p), "\n"), "\n", " ")
	payload := fmt.Sprintf("PRIORITY=6\nSYSLOG_IDENTIFIER=%s\nMESSAGE=%s\n", j.tag, msg)
	_, err := j.conn.Write([]byte(payload))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
