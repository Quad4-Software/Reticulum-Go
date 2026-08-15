// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sandbox

import (
	"errors"
	"fmt"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestWrapSandboxError(t *testing.T) {
	inner := errors.New("landlock denied")
	err := fmt.Errorf("%w: %w", common.ErrSandbox, inner)
	if !errors.Is(err, common.ErrSandbox) {
		t.Fatalf("wrap must preserve ErrSandbox, got %v", err)
	}
	if !errors.Is(err, inner) {
		t.Fatalf("wrap must preserve inner error, got %v", err)
	}
}
