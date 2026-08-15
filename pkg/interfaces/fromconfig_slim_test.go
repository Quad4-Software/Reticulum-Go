// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build rns_slim && !js

package interfaces

import (
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestSlimOmitsOptionalInterfaces(t *testing.T) {
	types := []string{
		"QUICClientInterface",
		"QUICServerInterface",
		"I2PInterface",
		"SDRInterface",
		"WebTransportClientInterface",
		"WebTransportServerInterface",
	}
	for _, typeName := range types {
		_, err := NewFromConfig("x", &common.InterfaceConfig{Type: typeName, Enabled: false})
		if err == nil || !strings.Contains(err.Error(), "rns_slim") {
			t.Fatalf("%s: expected slim omission error, got %v", typeName, err)
		}
	}
}

func TestSlimKeepsTCPUDP(t *testing.T) {
	udp, err := NewFromConfig("u", &common.InterfaceConfig{
		Type:    "UDPInterface",
		Enabled: false,
		Address: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if udp.GetType() != common.IFTypeUDP {
		t.Fatalf("type=%v", udp.GetType())
	}
}
