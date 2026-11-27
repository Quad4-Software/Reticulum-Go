// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build goexperiment.runtimesecret

package securemem

import "runtime/secret"

// Do wraps runtime/secret.Do: f runs, then the runtime erases secret data
// left in registers and on the stack. Only linux/amd64 and linux/arm64 have
// a real implementation. Other targets fall back to a plain call.
func Do(f func()) { secret.Do(f) }
