// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !goexperiment.runtimesecret

package securemem

// Do runs f. When built with -tags goexperiment.runtimesecret on a supported
// platform it wraps runtime/secret.Do so register and stack copies of secret
// material are erased when f returns; otherwise it calls f directly.
func Do(f func()) { f() }
