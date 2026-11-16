// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build js || wasm || plan9 || haiku

package sandbox

import (
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
)

func applyPlatform(cfg *common.ReticulumConfig) error {
	debug.Log(debug.DebugInfo, "Sandbox not supported on this platform, skipping")
	return nil
}
