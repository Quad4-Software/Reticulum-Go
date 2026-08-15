// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestNewFromConfigUnsupported(t *testing.T) {
	_, err := NewFromConfig("x", &common.InterfaceConfig{Type: "NoSuchInterface", Enabled: true})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported type error, got %v", err)
	}
}

func TestNewFromConfigNil(t *testing.T) {
	_, err := NewFromConfig("x", nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewFromConfigModem73(t *testing.T) {
	m73, err := NewFromConfig("m", &common.InterfaceConfig{
		Type:        "Modem73Interface",
		Enabled:     false,
		TargetHost:  "127.0.0.1",
		TargetPort:  8001,
		ControlHost: "127.0.0.1",
		ControlPort: 8073,
		ShortFrames: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m73.GetType() != common.IFTypeModem73 {
		t.Fatalf("type=%v", m73.GetType())
	}
}
