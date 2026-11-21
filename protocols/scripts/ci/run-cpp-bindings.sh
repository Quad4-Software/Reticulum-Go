#!/bin/sh
# SPDX-License-Identifier: LicenseRef-Reticulum
set -eu
ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
task build-librrc
task build-libmf
task build-liblxmf
task build-liblxst
make -C bindings/cpp test
