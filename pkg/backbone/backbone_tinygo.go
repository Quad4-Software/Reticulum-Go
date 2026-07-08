// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build tinygo

package backbone

func defaultBackend() Backend {
	return BackendGo
}
