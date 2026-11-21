// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package sandbox

import (
	"fmt"
	"runtime"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
)

// Apply installs platform sandbox restrictions when cfg is nil or
// cfg.EnableSandbox is true. Returns ErrSandbox when the platform apply fails.
func Apply(cfg *common.ReticulumConfig) error {
	SetExecRlimits(cfg != nil && cfg.SandboxExecRlimits)
	if cfg != nil && !cfg.EnableSandbox {
		debug.Log(debug.DebugInfo, "Sandbox disabled by configuration")
		return nil
	}
	debug.Log(debug.DebugInfo, "Applying sandbox", "platform", runtime.GOOS, "arch", runtime.GOARCH)
	if err := applyPlatform(cfg); err != nil {
		return fmt.Errorf("%w: %w", common.ErrSandbox, err)
	}
	return nil
}
