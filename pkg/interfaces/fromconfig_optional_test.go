// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !rns_slim

package interfaces

import (
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
)

func TestNewFromConfigSDR(t *testing.T) {
	sdrIface, err := NewFromConfig("s", &common.InterfaceConfig{
		Type:    "SDRInterface",
		Enabled: false,
		Device:  "mock",
		Modem:   "burst",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sdrIface.GetType() != common.IFTypeSDR {
		t.Fatalf("type=%v", sdrIface.GetType())
	}
}
