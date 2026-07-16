// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package debug

import (
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestSplitLogDestinations(t *testing.T) {
	got := splitLogDestinations("syslog+stderr")
	if len(got) != 2 || got[0] != "syslog" || got[1] != "stderr" {
		t.Fatalf("got %#v", got)
	}
	got = splitLogDestinations("journald,file")
	if len(got) != 2 || got[0] != "journald" || got[1] != "file" {
		t.Fatalf("got %#v", got)
	}
}

func TestConfigureDestinationSyslogParses(t *testing.T) {
	Init()
	cfg := &common.ReticulumConfig{LogDestination: "stderr"}
	if err := ConfigureDestination(cfg); err != nil {
		t.Fatal(err)
	}
	// syslog may fail in restricted environments without a socket
	cfg.LogDestination = "syslog"
	err := ConfigureDestination(cfg)
	if err != nil && !strings.Contains(err.Error(), "syslog") {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = ConfigureDestination(&common.ReticulumConfig{LogDestination: "stderr"})
}
