// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/transport"
)

func TestWriteStatusHumanShowsBlockedIPs(t *testing.T) {
	clients := 2
	blocked := 1
	var buf bytes.Buffer
	stats := transport.InterfaceStatsResponse{
		Interfaces: []transport.InterfaceStat{{
			Name:          "BackboneInterface[test]",
			Status:        true,
			Clients:       &clients,
			BlockedIPs:    &blocked,
			BlockedIPList: []string{"198.51.100.9"},
		}},
	}
	if err := WriteStatusHuman(&buf, stats, nil, nil, StatusOptions{ShowAll: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Clients   : 2") {
		t.Fatalf("missing clients line: %s", out)
	}
	if !strings.Contains(out, "Blocked   : 1 IPs") {
		t.Fatalf("missing blocked line: %s", out)
	}
}
