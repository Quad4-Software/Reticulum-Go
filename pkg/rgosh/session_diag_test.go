// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rgosh

import "testing"

func TestSessionStreamDiagnostics(t *testing.T) {
	s := NewSession(Config{}, nil)
	defer s.Close()
	if s.ExitReceived() || s.StdoutEOF() || s.StderrEOF() {
		t.Fatal("new session reports complete streams")
	}
	code := 0
	s.clientExitCode = &code
	s.clientStdoutEOF = true
	s.clientStderrEOF = true
	if !s.ExitReceived() || !s.StdoutEOF() || !s.StderrEOF() {
		t.Fatal("session diagnostics do not report arrived stream state")
	}
}
