// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"crypto/sha256"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
)

// pythonAutoHash runs the Python reference hash generation for the given
// group_id and peer_ip and returns the hex-encoded hash.
func pythonAutoHash(t *testing.T, groupID, peerIP string) string {
	t.Helper()
	script := fmt.Sprintf(
		"import sys; sys.path.insert(0, 'reticulum-ref'); import RNS; "+
			"print(RNS.Identity.full_hash(%q.encode('utf-8') + %q.encode('utf-8')).hex())",
		groupID, peerIP,
	)
	cmd := exec.Command("python3", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python hash failed: %v\noutput: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// pythonAutoMcast runs the Python reference multicast address generation for
// the given group_id, scope and addr_type.
func pythonAutoMcast(t *testing.T, groupID, scope, addrType string) string {
	t.Helper()
	script := fmt.Sprintf(
		"import sys; sys.path.insert(0, 'reticulum-ref'); import RNS; "+
			"g = RNS.Identity.full_hash(%q.encode('utf-8')); "+
			"gt = '0'; "+
			"gt += ':'+'{:02x}'.format(g[3]+(g[2]<<8)); "+
			"gt += ':'+'{:02x}'.format(g[5]+(g[4]<<8)); "+
			"gt += ':'+'{:02x}'.format(g[7]+(g[6]<<8)); "+
			"gt += ':'+'{:02x}'.format(g[9]+(g[8]<<8)); "+
			"gt += ':'+'{:02x}'.format(g[11]+(g[10]<<8)); "+
			"gt += ':'+'{:02x}'.format(g[13]+(g[12]<<8)); "+
			"print('ff'+ %q + %q + ':' + gt)",
		groupID, addrType, scope,
	)
	cmd := exec.Command("python3", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python mcast failed: %v\noutput: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestAutoInteropDiscoveryHash verifies that Go and Python generate the same
// discovery authentication hash.
func TestAutoInteropDiscoveryHash(t *testing.T) {
	cases := []struct {
		groupID string
		peerIP  string
	}{
		{"reticulum", "fe80::1"},
		{"reticulum", "fe80::abcd:1234"},
		{"customGroup", "fe80::2"},
		{"", "fe80::1"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("group=%s_peer=%s", tc.groupID, tc.peerIP), func(t *testing.T) {
			effectiveGroup := tc.groupID
			if effectiveGroup == "" {
				effectiveGroup = DefaultGroupID
			}
			pyHash := pythonAutoHash(t, effectiveGroup, tc.peerIP)

			tokenSource := append([]byte(effectiveGroup), []byte(tc.peerIP)...)
			goHash := fmt.Sprintf("%x", sha256Hash(tokenSource))

			if goHash != pyHash {
				t.Errorf("hash mismatch: go=%s py=%s", goHash, pyHash)
			}
		})
	}
}

// TestAutoInteropMcastAddress verifies that Go and Python generate the same
// multicast discovery address.
func TestAutoInteropMcastAddress(t *testing.T) {
	cases := []struct {
		groupID  string
		scope    string
		addrType string
	}{
		{"reticulum", "2", "1"},
		{"reticulum", "2", "0"},
		{"customGroup", "2", "1"},
		{"reticulum", "5", "1"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("group=%s_scope=%s_type=%s", tc.groupID, tc.scope, tc.addrType), func(t *testing.T) {
			pyMcast := pythonAutoMcast(t, tc.groupID, tc.scope, tc.addrType)

			config := &common.InterfaceConfig{
				Enabled:           true,
				GroupID:           tc.groupID,
				DiscoveryScope:    tc.scope,
				MulticastAddrType: tc.addrType,
			}
			ai, err := NewAutoInterface("test", config)
			if err != nil {
				t.Fatalf("NewAutoInterface failed: %v", err)
			}

			if ai.mcastDiscoveryAddr != pyMcast {
				t.Errorf("mcast address mismatch: go=%s py=%s", ai.mcastDiscoveryAddr, pyMcast)
			}
		})
	}
}

// sha256Hash is a thin helper so the test doesn't depend on the exact
// crypto/sha256 import pattern used by the implementation.
func sha256Hash(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}
