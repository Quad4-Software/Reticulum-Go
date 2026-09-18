#!/bin/sh
# SPDX-License-Identifier: LicenseRef-Reticulum
# Copyright (c) 2024-2026 Quad4.io
#
# CGO CC wrapper: Zig as x86_64-windows-gnu cross compiler.

exec zig cc -target x86_64-windows-gnu "$@"
