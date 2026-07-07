//go:build race

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

// raceBuild reports whether the test binary was built with -race. See the
// non-race variant of this file for details.
const raceBuild = true
