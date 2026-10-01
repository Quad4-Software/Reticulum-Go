// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

func TestWriteStatusHumanShowsAnnounceReasons(t *testing.T) {
	var buf bytes.Buffer
	stats := transport.InterfaceStatsResponse{
		Interfaces: []transport.InterfaceStat{{
			Name:               "UDPInterface[test]",
			Status:             true,
			AnnounceOK:         42,
			AnnounceDup:        3,
			AnnounceHeld:       2,
			AnnounceSigFail:    5,
			AnnounceMalformed:  1,
			AnnounceBlackholed: 7,
			AnnounceMaxHops:    4,
		}},
	}
	if err := WriteStatusHuman(&buf, stats, nil, nil, StatusOptions{ShowAll: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "ok=42 dup=3 held=2") {
		t.Fatalf("missing announce counters: %s", out)
	}
	for _, want := range []string{"sig=5", "malformed=1", "blackholed=7", "max_hops=4"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing reject reason %q: %s", want, out)
		}
	}
}

func TestWriteStatusHumanShowsEndpointQuarantine(t *testing.T) {
	var buf bytes.Buffer
	stats := transport.InterfaceStatsResponse{
		Interfaces: []transport.InterfaceStat{{
			Name:                 "TCPClientInterface[dead]",
			Status:               false,
			EndpointDialFailures: 8,
			EndpointFlaps:        2,
			EndpointQuarantined:  true,
			EndpointQuarantineS:  300,
		}},
	}
	if err := WriteStatusHuman(&buf, stats, nil, nil, StatusOptions{ShowAll: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Endpoint  :") {
		t.Fatalf("missing endpoint line: %s", out)
	}
	if !strings.Contains(out, "dial_failures=8") || !strings.Contains(out, "quarantined=true") || !strings.Contains(out, "quarantine_s=300") {
		t.Fatalf("incomplete endpoint line: %s", out)
	}
}

func TestWriteStatusHumanOmitsZeroEndpoint(t *testing.T) {
	var buf bytes.Buffer
	stats := transport.InterfaceStatsResponse{
		Interfaces: []transport.InterfaceStat{{Name: "UDPInterface[ok]", Status: true}},
	}
	if err := WriteStatusHuman(&buf, stats, nil, nil, StatusOptions{ShowAll: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "Endpoint  :") {
		t.Fatalf("endpoint line should be hidden when zero: %s", buf.String())
	}
}

func TestWriteStatusJSONCarriesNewFields(t *testing.T) {
	var buf bytes.Buffer
	stats := transport.InterfaceStatsResponse{
		Interfaces: []transport.InterfaceStat{{
			Name:                "UDPInterface[test]",
			Status:              true,
			AnnounceMalformed:   2,
			AnnounceBlackholed:  1,
			EndpointQuarantined: true,
			EndpointQuarantineS: 60,
		}},
	}
	if err := WriteStatusJSON(&buf, stats); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"announce_malformed":2`,
		`"announce_blackholed":1`,
		`"endpoint_quarantined":true`,
		`"endpoint_quarantine_s":60`,
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("json missing %q: %s", want, buf.String())
		}
	}
}
