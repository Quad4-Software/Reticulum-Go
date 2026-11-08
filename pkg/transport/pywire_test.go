// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"encoding/hex"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
)

func TestPyStampedAnnounceWire(t *testing.T) {
	raw, _ := hex.DecodeString("0100130f20309929eb81aae1d28b89c4bcd00056548bfdf0f25db92da2461947d8057325fc4f1b644e6a75fdc4f6b1fafb3f1da3137b63db246eabf30015b2c2975585bfc9d9877f61e64016b2489989bb167fb50b1fe81c2b2105023fb5d03576a8006aefd5fe243fde2cd804f824409fa5238d7684fcfb5764cf8cdad6c2b742a50ebc3f5bb92ecf86c085eb6ae705f322a1faf23ed3595f1e8bdbc6a648154f918c4db3b00ec100221019ec4c6916a32fb316c0d7a6800c")
	tr, iface, h := newStampTestTransport(t, common.StampsConfig{DestinationCost: 8, DestinationCostSet: true})
	if err := tr.handleAnnouncePacket(raw, iface); err != nil {
		t.Fatalf("handleAnnouncePacket: %v", err)
	}
	if h.calls != 1 {
		t.Fatalf("handler calls=%d", h.calls)
	}
	destHash, _ := hex.DecodeString("130f20309929eb81aae1d28b89c4bcd0")
	if sv, ok := identity.RecallStampValue(destHash); !ok || sv < 8 {
		t.Fatalf("stamp value %v ok=%v", sv, ok)
	}
}
