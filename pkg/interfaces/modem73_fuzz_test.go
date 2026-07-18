// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func FuzzModem73ControlFrame(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 0, 0, 2, '{', '}'})
	frame, _ := modem73EncodeControl(map[string]any{"cmd": "get_config"})
	f.Add(frame)
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = modem73ReadControl(bytes.NewReader(data))
		if len(data) >= 4 {
			n := binary.BigEndian.Uint32(data[:4])
			if n > modem73MaxControlJSON {
				return
			}
		}
	})
}

func FuzzModem73ShortFrameDecision(f *testing.F) {
	f.Add([]byte("hi"), uint8(0), uint8(10))
	f.Fuzz(func(t *testing.T, payload []byte, policyByte uint8, shortMTU uint8) {
		policy := []string{"off", "auto", "always"}[int(policyByte)%3]
		mtu := int(shortMTU)
		if mtu == 0 {
			mtu = 170
		}
		useShort := policy == "auto" && len(payload) <= mtu
		_ = useShort
		if policy == "off" && useShort {
			t.Fatal("off must not use short")
		}
	})
}
