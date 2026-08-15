#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# CGO CC wrapper: Zig as armv6 soft-float Linux (Pi Zero / Pi 1).

exec zig cc -target arm-linux-gnueabi -mcpu=arm1176jzf_s "$@"
