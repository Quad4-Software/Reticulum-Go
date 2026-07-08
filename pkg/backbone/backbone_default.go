// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !tinygo

package backbone

import "runtime"

func defaultBackend() Backend {
	switch runtime.GOOS {
	case "linux", "android":
		return BackendEpoll
	case "darwin", "freebsd", "netbsd", "openbsd":
		return BackendKqueue
	default:
		return BackendGo
	}
}
