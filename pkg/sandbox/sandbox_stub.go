// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build js || wasm || plan9

package sandbox

import (
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
)

func applyPlatform(cfg *common.ReticulumConfig) error {
	debug.Log(debug.DebugInfo, "Sandbox not supported on this platform, skipping")
	return nil
}
