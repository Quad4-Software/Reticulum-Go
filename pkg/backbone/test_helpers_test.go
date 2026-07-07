// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package backbone

import (
	"runtime"
	"testing"
)

func testHubWithBackend(t *testing.T, backend Backend) *Hub {
	t.Helper()
	Shutdown()
	hub, err := Init(backend)
	if err != nil {
		t.Fatalf("Init(%s): %v", backend, err)
	}
	t.Cleanup(Shutdown)
	return hub
}

func testableBackends(t *testing.T) []Backend {
	t.Helper()
	backends := []Backend{BackendGo}
	switch runtime.GOOS {
	case "linux", "android":
		backends = append(backends, BackendEpoll)
		if hub, err := Init(BackendUring); err == nil {
			_ = hub
			Shutdown()
			backends = append(backends, BackendUring)
		} else {
			Shutdown()
		}
	case "darwin", "freebsd", "netbsd", "openbsd":
		backends = append(backends, BackendKqueue)
	}
	return backends
}
