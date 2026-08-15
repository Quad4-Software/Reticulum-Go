#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# Reload systemd after install or upgrade. Do not enable or start the daemon.
# Arch nfpm wraps this as a function so this file must not call exit.

if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload >/dev/null 2>&1 || true
fi
