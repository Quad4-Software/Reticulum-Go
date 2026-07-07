// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestUpdateLinkLocalAddressesEmpty(t *testing.T) {
	ai, err := NewAutoInterface("auto", &common.InterfaceConfig{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	ai.updateLinkLocalAddresses()
}
