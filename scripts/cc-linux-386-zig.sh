#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# CGO CC wrapper: Zig as x86-linux-gnu cross compiler.

exec zig cc -target x86-linux-gnu "$@"
