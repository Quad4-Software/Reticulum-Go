#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# CGO CC wrapper: Zig as arm-linux-gnueabihf (ARMv7 hard-float).

exec zig cc -target arm-linux-gnueabihf "$@"
