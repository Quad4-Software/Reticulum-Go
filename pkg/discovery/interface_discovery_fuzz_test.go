// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"testing"
)

func FuzzInterfaceDiscoveryReceivedAnnounce(f *testing.F) {
	good, err := BuildAppData(Info{
		Type:        "AutoInterface",
		TransportID: bytes.Repeat([]byte{0xab}, 16),
		Name:        "fuzz",
	}, 2, WorkblockExpandRounds)
	if err == nil {
		f.Add(good)
	}
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0xff, 0xff})
	f.Add(bytes.Repeat([]byte{0x11}, 128))

	f.Fuzz(func(t *testing.T, app []byte) {
		if len(app) > 1<<16 {
			t.Skip()
		}
		h := &interfaceAnnounceHandler{
			requiredValue: 2,
			validCache:    make(map[string]*ReceivedAnnounceInfo),
			invalidCache:  make(map[string]struct{}),
		}
		_ = h.ReceivedAnnounce(nil, nil, app, 0)
		if len(h.validCache) > 1 || len(h.invalidCache) > 1 {
			t.Fatalf("single announce produced multiple cache entries valid=%d invalid=%d",
				len(h.validCache), len(h.invalidCache))
		}
	})
}
