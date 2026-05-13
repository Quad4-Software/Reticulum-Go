// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sandbox

import (
	"runtime"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
)

func Apply(cfg *common.ReticulumConfig) error {
	if cfg != nil && !cfg.EnableSandbox {
		debug.Log(debug.DebugInfo, "Sandbox disabled by configuration")
		return nil
	}
	debug.Log(debug.DebugInfo, "Applying sandbox", "platform", runtime.GOOS, "arch", runtime.GOARCH)
	return applyPlatform(cfg)
}
