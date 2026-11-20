// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"strings"
	"testing"
)

func FuzzParseBlockedIPs(f *testing.F) {
	f.Add([]byte("1.2.3.4\n# comment\n\n::1\n"))
	f.Add([]byte("not-an-ip\n10.0.0.1 \n  fe80::1\n"))
	f.Add([]byte{0xff, 0x00, '\n', 0x01})
	f.Fuzz(func(t *testing.T, raw []byte) {
		for _, ip := range parseBlockedIPs(raw) {
			if strings.TrimSpace(ip) != ip || ip == "" || strings.HasPrefix(ip, "#") {
				t.Fatalf("invalid entry %q returned", ip)
			}
		}
	})
}
