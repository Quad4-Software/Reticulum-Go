// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build windows || js

package rgosh

import "os"

// waitStatusCode has no signaled-waitstatus semantics on this platform; the
// reported exit code is used as-is.
func waitStatusCode(_ *os.ProcessState, fallback int) int { return fallback }
