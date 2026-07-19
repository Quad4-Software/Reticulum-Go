// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sandbox

import (
	"fmt"
	"runtime"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

// Apply installs platform sandbox restrictions when cfg is nil or
// cfg.EnableSandbox is true. Returns ErrSandbox when the platform apply fails.
func Apply(cfg *common.ReticulumConfig) error {
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
