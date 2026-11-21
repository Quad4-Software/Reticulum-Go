#!/bin/sh
# SPDX-License-Identifier: LicenseRef-Reticulum
set -eu
ROOT="$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
task build-librrc
go test -count=1 -timeout 2m ./pkg/librrc/... -run TestHubClientLoopback
make -C bindings/python examples/hub-client
make -C bindings/rust examples/hub-client
make -C bindings/java examples/hub-client
make -C bindings/lua examples/hub-client
make -C bindings/c examples/hub-client
