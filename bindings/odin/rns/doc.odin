// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns

/*
Odin bindings for the librns C ABI.

Link against bin/librns.so via system:rns. Keep include/rns.h as the ABI source of truth.
Idiomatic wrappers sit beside the foreign declarations so hosts can use Odin types
while still calling the raw rns entry points when needed.
*/
