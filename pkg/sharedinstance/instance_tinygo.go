// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build tinygo

package sharedinstance

import (
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/transport"
)

// Attach is a no-op on TinyGo: local shared-instance sockets are not supported.
func Attach(cfg *common.ReticulumConfig, tr *transport.Transport, hooks Hooks) (*Instance, error) {
	_ = tr
	_ = hooks
	if cfg != nil && cfg.ShareInstance {
		cfg.ConnectedToSharedInstance = false
	}
	return &Instance{Mode: ModeDisabled}, nil
}
