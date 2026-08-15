// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build js || wasm || plan9 || haiku || tinygo

package sandbox

import (
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

func applyPlatform(cfg *common.ReticulumConfig) error {
	debug.Log(debug.DebugInfo, "Sandbox not supported on this platform, skipping")
	return nil
}
