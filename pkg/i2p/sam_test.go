// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package i2p

import "testing"

func TestParseMessage(t *testing.T) {
	m, err := parseMessage("STREAM STATUS RESULT=OK\n")
	if err != nil {
		t.Fatal(err)
	}
	if m.Cmd != "STREAM" || m.Action != "STATUS" || !m.OK() {
		t.Fatalf("unexpected message: %+v", m)
	}
}

func TestParseMessageError(t *testing.T) {
	m, err := parseMessage("STREAM STATUS RESULT=TIMEOUT\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.ResultError(); err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestSAMAddressFromEnv(t *testing.T) {
	t.Setenv("I2P_SAM_ADDRESS", "127.0.0.1:9999")
	if got := SAMAddressFromEnv(); got != "127.0.0.1:9999" {
		t.Fatalf("got %q", got)
	}
}
