// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
)

// TestOnNetworkAvailableResetsEndpointQuarantines verifies the
// host-signalled network-return path clears DefaultEndpointTracker so
// quarantined interfaces dial immediately.
func TestOnNetworkAvailableResetsEndpointQuarantines(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = n.Stop() }()

	tr := interfaces.DefaultEndpointTracker
	t.Cleanup(tr.ResetQuarantines)
	ep := "lifecycle-quarantine:4242"
	for i := 0; i < interfaces.EndpointQuarantineConsecutiveFails; i++ {
		tr.RecordFailure(ep)
	}
	if ok, _ := tr.DialAllowed(ep); ok {
		t.Fatal("expected quarantine")
	}

	if err := n.OnNetworkAvailable(); err != nil {
		t.Fatal(err)
	}
	if ok, _ := tr.DialAllowed(ep); !ok {
		t.Fatal("OnNetworkAvailable must clear endpoint quarantines")
	}
}
