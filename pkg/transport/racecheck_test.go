//go:build !race

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

// raceBuild reports whether the test binary was built with -race. The race
// detector's instrumented allocator adds per-object bookkeeping that
// inflates runtime.MemStats.Alloc independently of actual application
// memory efficiency, so byte-level footprint budgets are only meaningful on
// a non-instrumented build.
const raceBuild = false
